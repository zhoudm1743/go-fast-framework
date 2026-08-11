package queue

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

const defaultQueueSize = 1024 // 有界队列缓冲上限

// Option 队列管理器配置项。
type Option func(*manager)

// WithWorkers 设置后台 worker 数量（>0 才生效）。
func WithWorkers(n int) Option {
	return func(m *manager) {
		if n > 0 {
			m.workers = n
		}
	}
}

// manager 队列管理器（同步驱动 + 有界 worker 池）。
type manager struct {
	log     contracts.Log
	workers int
	jobCh   chan *pendingTask
	stopCh  chan struct{}
	wg      sync.WaitGroup
	mu      sync.Mutex
	stopped bool
}

// New 创建队列管理器；log 用于输出任务失败/重试日志，不能为 nil。
func New(log contracts.Log, opts ...Option) contracts.Queue {
	m := &manager{
		log:     log,
		workers: runtime.NumCPU(),
		jobCh:   make(chan *pendingTask, defaultQueueSize),
		stopCh:  make(chan struct{}),
	}
	for _, o := range opts {
		o(m)
	}
	m.startWorkers()
	return m
}

func (m *manager) startWorkers() {
	for i := 0; i < m.workers; i++ {
		m.wg.Add(1)
		go m.worker()
	}
}

// worker 消费任务队列并执行；range 在 jobCh 关闭并排空后退出。
func (m *manager) worker() {
	defer m.wg.Done()
	for task := range m.jobCh {
		if err := task.run(); err != nil {
			m.log.WithError(err).
				WithField("queue", task.queue).
				WithField("connection", task.connection).
				WithField("job", task.signatures()).
				Error("[GoFast] 队列任务执行失败")
		}
	}
}

// enqueue 投递任务；队列关闭时返回错误。缓冲满时阻塞（背压）。
func (m *manager) enqueue(task *pendingTask) error {
	select {
	case <-m.stopCh:
		return fmt.Errorf("[GoFast] queue: 队列已关闭，无法派发新任务")
	default:
	}
	select {
	case m.jobCh <- task:
		return nil
	case <-m.stopCh:
		return fmt.Errorf("[GoFast] queue: 队列已关闭，无法派发新任务")
	}
}

// Stop 优雅关闭：停止接收新任务，排空既有任务后退出 worker。
func (m *manager) Stop() {
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return
	}
	m.stopped = true
	close(m.stopCh)
	close(m.jobCh)
	m.mu.Unlock()
	m.wg.Wait()
}

func (m *manager) Job(job contracts.QueueJob, args []contracts.QueueArg) contracts.QueuePending {
	return &pendingTask{
		mgr:  m,
		jobs: []contracts.QueueChain{{Job: job, Args: args}},
	}
}

func (m *manager) Chain(jobs []contracts.QueueChain) contracts.QueuePending {
	return &pendingTask{mgr: m, jobs: jobs}
}

// ── pendingTask ──────────────────────────────────────────────────────

type pendingTask struct {
	mgr        *manager
	jobs       []contracts.QueueChain
	queue      string
	connection string
	delay      time.Time
	retry      int
}

func (p *pendingTask) OnQueue(queue string) contracts.QueuePending {
	p.queue = queue
	return p
}

func (p *pendingTask) OnConnection(connection string) contracts.QueuePending {
	p.connection = connection
	return p
}

func (p *pendingTask) Delay(delay time.Time) contracts.QueuePending {
	p.delay = delay
	return p
}

func (p *pendingTask) Retry(times int) contracts.QueuePending {
	p.retry = times
	return p
}

// Dispatch 投递到后台 worker 池异步执行。
func (p *pendingTask) Dispatch() error {
	return p.mgr.enqueue(p)
}

// DispatchSync 同步立即执行，不走队列，忽略 Delay，仍应用 Retry。
func (p *pendingTask) DispatchSync() error {
	return p.execWithRetry()
}

// run 异步路径：先处理延迟，再执行（含重试）。
func (p *pendingTask) run() error {
	if !p.delay.IsZero() {
		if wait := time.Until(p.delay); wait > 0 {
			time.Sleep(wait)
		}
	}
	return p.execWithRetry()
}

// execWithRetry 按重试次数执行任务链；panic 由 execChainWithRecover 转为 error。
func (p *pendingTask) execWithRetry() error {
	attempts := p.retry + 1
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 {
			time.Sleep(retryBackoff(attempt))
		}
		err = p.execChainWithRecover()
		if err == nil {
			return nil
		}
		if attempt < attempts {
			p.mgr.log.WithError(err).
				WithField("job", p.signatures()).
				WithField("attempt", attempt).
				Warn("[GoFast] 队列任务执行失败，即将重试")
		}
	}
	return err
}

// execChainWithRecover 执行任务链；panic 时转换为 error。
func (p *pendingTask) execChainWithRecover() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("[GoFast] queue job %v panic: %v", p.signatures(), r)
		}
	}()
	return executeChain(p.jobs)
}

func (p *pendingTask) signatures() []string {
	sigs := make([]string, len(p.jobs))
	for i, c := range p.jobs {
		sigs[i] = c.Job.Signature()
	}
	return sigs
}

func retryBackoff(attempt int) time.Duration {
	return time.Duration(attempt-1) * 100 * time.Millisecond
}

// ── 公共函数 ─────────────────────────────────────────────────────────

// executeChain 按顺序执行任务链；任一失败则终止。
func executeChain(chain []contracts.QueueChain) error {
	for _, item := range chain {
		anyArgs := argsToAny(item.Args)
		if err := item.Job.Handle(anyArgs...); err != nil {
			return fmt.Errorf("[GoFast] queue job %s failed: %w", item.Job.Signature(), err)
		}
	}
	return nil
}

// argsToAny 将 QueueArg 转为 any 切片。
// 注意：QueueArg.Type 仅作元数据描述，实际按位置顺序传参；
// Handle(args ...any) 本身无类型校验，这是 Go 语言的限制。
func argsToAny(args []contracts.QueueArg) []any {
	result := make([]any, len(args))
	for i, a := range args {
		result[i] = a.Value
	}
	return result
}
