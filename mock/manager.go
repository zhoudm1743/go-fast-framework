package mock

import (
	"sync"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/foundation"
)

// manager 实现 contracts.MockManager。
type manager struct {
	app      foundation.Application
	mu       sync.Mutex
	original map[string]*originalEntry
}

type originalEntry struct {
	bound    bool
	instance any
}

// NewManager 创建 MockManager 实例。
func NewManager(app foundation.Application) contracts.MockManager {
	return &manager{
		app:      app,
		original: make(map[string]*originalEntry),
	}
}

func (m *manager) Swap(key string, instance any) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.original[key]; !ok {
		entry := &originalEntry{bound: m.app.Bound(key)}
		if entry.bound {
			entry.instance, _ = m.app.Make(key)
		}
		m.original[key] = entry
	}

	m.app.Instance(key, instance)
}

func (m *manager) Restore(keys ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(keys) == 0 {
		return
	}

	for _, key := range keys {
		if entry, ok := m.original[key]; ok {
			if entry.bound {
				m.app.Instance(key, entry.instance)
			} else {
				m.app.Unbind(key)
			}
			delete(m.original, key)
		}
	}
}

func (m *manager) RestoreAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for key, entry := range m.original {
		if entry.bound {
			m.app.Instance(key, entry.instance)
		} else {
			m.app.Unbind(key)
		}
	}
	m.original = make(map[string]*originalEntry)
}

func (m *manager) Cache() contracts.Cache {
	if c, err := m.app.Make("cache"); err == nil {
		if mc, ok := c.(*MockCache); ok {
			return mc
		}
	}
	mc := NewMockCache()
	m.Swap("cache", mc)
	return mc
}

func (m *manager) DB() contracts.DB {
	mc := NewMockDB()
	m.Swap("db", mc)
	return mc
}

func (m *manager) Queue() contracts.Queue {
	mq := NewMockQueue()
	m.Swap("queue", mq)
	return mq
}

func (m *manager) Event() contracts.Event {
	me := NewMockEvent()
	m.Swap("event", me)
	return me
}

func (m *manager) Storage() contracts.Storage {
	ms := NewMockStorage()
	m.Swap("storage", ms)
	return ms
}

func (m *manager) Log() contracts.Log {
	ml := NewMockLog()
	m.Swap("log", ml)
	return ml
}
