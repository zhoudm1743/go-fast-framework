package mock

import (
	"context"
	"sync"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// MockDB 实现 contracts.DB，用于测试。
type MockDB struct {
	mu sync.Mutex

	QueryFunc        func(ctx ...context.Context) contracts.Query
	TenantFunc       func(ctx contracts.Context) contracts.Query
	ConnectionFunc   func(name string) contracts.Query
	DriverFunc       func(name ...string) contracts.Driver
	TransactionFunc  func(fc func(tx contracts.Query) error, opts ...contracts.TxOption) error
	AutoMigrateFunc  func(models ...any) error
	PingFunc         func() error
	CloseFunc        func() error
	RegisterFunc     func(name string, cfg contracts.ConnectionConfig) error
}

// NewMockDB 创建 MockDB。
func NewMockDB() *MockDB {
	return &MockDB{}
}

func (m *MockDB) Query(ctx ...context.Context) contracts.Query {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.QueryFunc != nil {
		return m.QueryFunc(ctx...)
	}
	return NewMockQuery()
}

func (m *MockDB) Tenant(ctx contracts.Context) contracts.Query {
	if m.TenantFunc != nil {
		return m.TenantFunc(ctx)
	}
	return m.Query()
}

func (m *MockDB) Connection(name string) contracts.Query {
	if m.ConnectionFunc != nil {
		return m.ConnectionFunc(name)
	}
	return m.Query()
}

func (m *MockDB) Driver(name ...string) contracts.Driver {
	if m.DriverFunc != nil {
		return m.DriverFunc(name...)
	}
	return nil
}

func (m *MockDB) Transaction(fc func(tx contracts.Query) error, opts ...contracts.TxOption) error {
	if m.TransactionFunc != nil {
		return m.TransactionFunc(fc, opts...)
	}
	return fc(m.Query())
}

func (m *MockDB) AutoMigrate(models ...any) error {
	if m.AutoMigrateFunc != nil {
		return m.AutoMigrateFunc(models...)
	}
	return nil
}

func (m *MockDB) Ping() error {
	if m.PingFunc != nil {
		return m.PingFunc()
	}
	return nil
}

func (m *MockDB) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

func (m *MockDB) Register(name string, cfg contracts.ConnectionConfig) error {
	if m.RegisterFunc != nil {
		return m.RegisterFunc(name, cfg)
	}
	return nil
}
