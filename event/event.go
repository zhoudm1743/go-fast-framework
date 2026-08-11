package event

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// manager 事件总线实现。
type manager struct {
	mu       sync.RWMutex
	handlers map[string][]contracts.EventListener
	events   map[string]contracts.Eventer
	log      contracts.Log
	queueMgr contracts.Queue
}

// New 创建事件管理器。
func New() contracts.Event {
	return &manager{
		handlers: make(map[string][]contracts.EventListener),
		events:   make(map[string]contracts.Eventer),
	}
}

// SetLogger 注入日志服务，用于异步监听器的错误日志。
func (m *manager) SetLogger(l contracts.Log) {
	m.log = l
}

// SetQueue 注入队列服务。配置后，Enable=true 的监听器将通过队列异步执行，
// 替代原来的 goroutine 方式，获得持久化、重试等能力。
func (m *manager) SetQueue(q contracts.Queue) {
	m.queueMgr = q
}

func eventKey(e contracts.Eventer) string {
	t := reflect.TypeOf(e)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.PkgPath() + "." + t.Name()
}

func (m *manager) Register(events map[contracts.Eventer][]contracts.EventListener) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for e, listeners := range events {
		key := eventKey(e)
		m.events[key] = e
		m.handlers[key] = append(m.handlers[key], listeners...)
	}
}

func (m *manager) Job(event contracts.Eventer, args []contracts.EventArg) contracts.EventPending {
	return &pendingEvent{mgr: m, event: event, args: args}
}

// dispatch 执行事件派发（由 pendingEvent 调用）。
func (m *manager) dispatch(event contracts.Eventer, args []contracts.EventArg) error {
	key := eventKey(event)

	// 执行事件 Handle（数据加工）
	processed, err := event.Handle(args)
	if err != nil {
		return fmt.Errorf("[GoFast] event %s Handle error: %w", key, err)
	}

	m.mu.RLock()
	listeners := m.handlers[key]
	m.mu.RUnlock()

	// 将 []EventArg 转换为 []any 传给监听器
	anyArgs := make([]any, len(processed))
	for i, a := range processed {
		anyArgs[i] = a.Value
	}

	for _, l := range listeners {
		queue := l.Queue(anyArgs...)
		if queue.Enable {
			if m.queueMgr != nil {
				// 接入队列系统：持久化、重试、worker 池管理
				job := &listenerJob{listener: l}
				queueArgs := make([]contracts.QueueArg, len(anyArgs))
				for i, a := range anyArgs {
					queueArgs[i] = contracts.QueueArg{Value: a}
				}
				if err := m.queueMgr.Job(job, queueArgs).
					OnQueue(queue.Queue).
					OnConnection(queue.Connection).
					Dispatch(); err != nil {
					m.logError("event listener %s queue dispatch error: %v", l.Signature(), err)
				}
			} else {
				// 回退：goroutine（无队列系统时）
				go func(listener contracts.EventListener, a []any) {
					if err := listener.Handle(a...); err != nil {
						m.logError("event listener %s error: %v", listener.Signature(), err)
					}
				}(l, anyArgs)
			}
		} else {
			if err := l.Handle(anyArgs...); err != nil {
				// 同步模式：有错误则停止向后传播
				return fmt.Errorf("[GoFast] event listener %s error: %w", l.Signature(), err)
			}
		}
	}
	return nil
}

// ── pendingEvent ─────────────────────────────────────────────────────

type pendingEvent struct {
	mgr   *manager
	event contracts.Eventer
	args  []contracts.EventArg
}

func (p *pendingEvent) Dispatch() error {
	return p.mgr.dispatch(p.event, p.args)
}

func (m *manager) logError(format string, args ...any) {
	if m.log != nil {
		m.log.Errorf(format, args...)
		return
	}
	fmt.Printf("[GoFast] "+format+"\n", args...)
}

// ── listenerJob ──────────────────────────────────────────────────────

// listenerJob 将 EventListener 包装为 contracts.QueueJob，使事件监听器可通过队列系统调度。
type listenerJob struct {
	listener contracts.EventListener
}

func (j *listenerJob) Signature() string {
	return "event:" + j.listener.Signature()
}

func (j *listenerJob) Handle(args ...any) error {
	return j.listener.Handle(args...)
}
