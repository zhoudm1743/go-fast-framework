package mock

import (
	"sync"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// MockEvent 实现 contracts.Event，用于测试。
type MockEvent struct {
	mu sync.Mutex

	RegisterFunc func(events map[contracts.Eventer][]contracts.EventListener)
	JobFunc      func(event contracts.Eventer, args []contracts.EventArg) contracts.EventPending

	// Dispatched 记录所有派发过的事件。
	Dispatched []MockEventDispatch
}

// MockEventDispatch 记录一次事件派发。
type MockEventDispatch struct {
	Event contracts.Eventer
	Args  []contracts.EventArg
}

// NewMockEvent 创建 MockEvent。
func NewMockEvent() *MockEvent {
	return &MockEvent{Dispatched: make([]MockEventDispatch, 0)}
}

func (m *MockEvent) Register(events map[contracts.Eventer][]contracts.EventListener) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.RegisterFunc != nil {
		m.RegisterFunc(events)
		return
	}
}

func (m *MockEvent) Job(event contracts.Eventer, args []contracts.EventArg) contracts.EventPending {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.JobFunc != nil {
		return m.JobFunc(event, args)
	}
	m.Dispatched = append(m.Dispatched, MockEventDispatch{Event: event, Args: args})
	return &mockEventPending{dispatched: true}
}

type mockEventPending struct {
	dispatched bool
}

func (p *mockEventPending) Dispatch() error { p.dispatched = true; return nil }
