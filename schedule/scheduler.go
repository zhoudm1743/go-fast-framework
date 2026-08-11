package schedule

import (
	"fmt"
	"strings"
	"sync"

	"github.com/robfig/cron/v3"
	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// Scheduler 任务调度器实现。
type Scheduler struct {
	mu     sync.RWMutex
	cron   *cron.Cron
	events []*event
	cache  contracts.Cache
	kernel contracts.Fast
	log    contracts.Log
}

// New 创建调度器。
func New() *Scheduler {
	return &Scheduler{
		cron: cron.New(cron.WithSeconds()),
	}
}

// SetCache 注入缓存（OnOneServer 需要）。
func (s *Scheduler) SetCache(c contracts.Cache) {
	s.cache = c
}

// SetKernel 注入 Fast 内核（Command 调度需要）。
func (s *Scheduler) SetKernel(k contracts.Fast) {
	s.kernel = k
}

// SetLogger 注入日志服务。
func (s *Scheduler) SetLogger(l contracts.Log) {
	s.log = l
}

// RegisterEvents 批量注册调度任务（通常由用户在启动前调用）。
func (s *Scheduler) RegisterEvents(events []contracts.ScheduleEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range events {
		if ev, ok := e.(*event); ok {
			s.events = append(s.events, ev)
		}
	}
}

// Start 启动调度器（非阻塞）。
func (s *Scheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, ev := range s.events {
		ev.cache = s.cache
		ev.kernel = s.kernel
		ev.log = s.log

		cronExpr := normalizeCron(ev.cronExpr)
		if cronExpr == "" {
			s.logf("[GoFast] schedule: task %q has no cron expression, skipped", ev.name)
			continue
		}

		evCopy := ev // 避免闭包捕获循环变量
		if _, err := s.cron.AddFunc(cronExpr, evCopy.run); err != nil {
			return fmt.Errorf("[GoFast] schedule: add task %q failed: %w (expr=%q)", ev.name, err, cronExpr)
		}
	}
	s.cron.Start()
	return nil
}

// Stop 停止调度器。
func (s *Scheduler) Stop() {
	s.cron.Stop()
}

// List 返回所有已注册任务信息（名称 + cron 表达式）。
func (s *Scheduler) List() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]string, 0, len(s.events))
	for _, e := range s.events {
		name := e.name
		if name == "" {
			name = "(unnamed)"
		}
		result = append(result, fmt.Sprintf("%-30s %s", name, e.cronExpr))
	}
	return result
}

// ── Schedule 接口实现 ────────────────────────────────────────────────

func (s *Scheduler) Call(callback func()) contracts.ScheduleEvent {
	e := newEvent(callback, "")
	return e
}

func (s *Scheduler) Command(command string) contracts.ScheduleEvent {
	e := newEvent(nil, command)
	e.name = command
	return e
}

// normalizeCron 将 cron 表达式规范化为 robfig/cron 可用格式（WithSeconds 模式）。
// - "@every ..." / "@..." 直接传递
// - 5 字段（分钟级）：补 "0 " 前缀 → 变为 6 字段
// - 6 字段（秒级）：直接使用
func normalizeCron(expr string) string {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return ""
	}
	// 特殊描述符（@every, @daily 等）直接返回
	if strings.HasPrefix(expr, "@") {
		return expr
	}
	fields := strings.Fields(expr)
	switch len(fields) {
	case 5:
		return "0 " + expr
	case 6:
		return expr
	default:
		return expr
	}
}

func (s *Scheduler) logf(format string, args ...any) {
	if s.log != nil {
		s.log.Warnf(format, args...)
		return
	}
	fmt.Printf(format+"\n", args...)
}
