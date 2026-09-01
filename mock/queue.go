package mock

import (
	"sync"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// MockQueue 实现 contracts.Queue，用于测试。
type MockQueue struct {
	mu sync.Mutex

	JobFunc   func(job contracts.QueueJob, args []contracts.QueueArg) contracts.QueuePending
	ChainFunc func(jobs []contracts.QueueChain) contracts.QueuePending

	// Jobs 记录所有派发过的任务。
	Jobs []MockQueueJob
}

// MockQueueJob 记录一次任务派发。
type MockQueueJob struct {
	Job  contracts.QueueJob
	Args []contracts.QueueArg
}

// NewMockQueue 创建 MockQueue。
func NewMockQueue() *MockQueue {
	return &MockQueue{Jobs: make([]MockQueueJob, 0)}
}

func (m *MockQueue) Job(job contracts.QueueJob, args []contracts.QueueArg) contracts.QueuePending {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.JobFunc != nil {
		return m.JobFunc(job, args)
	}
	m.Jobs = append(m.Jobs, MockQueueJob{Job: job, Args: args})
	return &mockQueuePending{dispatched: true}
}

func (m *MockQueue) Chain(jobs []contracts.QueueChain) contracts.QueuePending {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ChainFunc != nil {
		return m.ChainFunc(jobs)
	}
	return &mockQueuePending{dispatched: true}
}

type mockQueuePending struct {
	dispatched bool
	delay      time.Time
	retry      int
}

func (p *mockQueuePending) OnQueue(queue string) contracts.QueuePending { return p }
func (p *mockQueuePending) OnConnection(connection string) contracts.QueuePending { return p }
func (p *mockQueuePending) Delay(delay time.Time) contracts.QueuePending {
	p.delay = delay
	return p
}
func (p *mockQueuePending) Retry(times int) contracts.QueuePending {
	p.retry = times
	return p
}
func (p *mockQueuePending) Dispatch() error     { p.dispatched = true; return nil }
func (p *mockQueuePending) DispatchSync() error { p.dispatched = true; return nil }
