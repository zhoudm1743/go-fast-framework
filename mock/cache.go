package mock

import (
	"sync"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// MockCache 实现 contracts.Cache 与 contracts.CacheStore，用于测试。
type MockCache struct {
	mu sync.Mutex

	GetFunc              func(key string, def ...any) any
	GetBoolFunc          func(key string, def ...bool) bool
	GetIntFunc           func(key string, def ...int) int
	GetInt64Func         func(key string, def ...int64) int64
	GetFloat64Func       func(key string, def ...float64) float64
	GetStringFunc        func(key string, def ...string) string
	HasFunc              func(key string) bool
	PutFunc              func(key string, value any, ttl time.Duration) error
	ForeverFunc          func(key string, value any) error
	ForgetFunc           func(key string) error
	FlushFunc            func() error
	PullFunc             func(key string, def ...any) any
	IncrementFunc        func(key string, value ...int64) (int64, error)
	DecrementFunc        func(key string, value ...int64) (int64, error)
	RememberFunc         func(key string, ttl time.Duration, callback func() (any, error)) (any, error)
	RememberForeverFunc  func(key string, callback func() (any, error)) (any, error)
	ManyFunc             func(keys []string) map[string]any
	PutManyFunc          func(values map[string]any, ttl time.Duration) error
	TagsFunc             func(tags ...string) contracts.TaggedCache
	HGetFunc             func(key, field string) (any, error)
	HSetFunc             func(key, field string, value any) error
	HDelFunc             func(key string, fields ...string) error
	HExistsFunc          func(key, field string) bool
	HGetAllFunc          func(key string) (map[string]any, error)
	HLenFunc             func(key string) int64
	HKeysFunc            func(key string) ([]string, error)
	LockFunc             func(key string, ttl time.Duration) contracts.CacheLock
	StoreFunc            func(name string) contracts.CacheStore

	// Data 可用来直接读写底层缓存数据（简化测试）。
	Data map[string]any
	// Calls 记录方法调用。
	Calls []Call
}

// Call 记录一次方法调用。
type Call struct {
	Method string
	Args   []any
}

// NewMockCache 创建默认的内存 MockCache。
func NewMockCache() *MockCache {
	return &MockCache{
		Data:  make(map[string]any),
		Calls: make([]Call, 0),
	}
}

func (m *MockCache) record(method string, args ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Calls = append(m.Calls, Call{Method: method, Args: args})
}

func (m *MockCache) Get(key string, def ...any) any {
	m.record("Get", key, def)
	if m.GetFunc != nil {
		return m.GetFunc(key, def...)
	}
	if v, ok := m.Data[key]; ok {
		return v
	}
	if len(def) > 0 {
		return def[0]
	}
	return nil
}

func (m *MockCache) GetBool(key string, def ...bool) bool {
	m.record("GetBool", key, def)
	if m.GetBoolFunc != nil {
		return m.GetBoolFunc(key, def...)
	}
	return m.Get(key, def).(bool)
}

func (m *MockCache) GetInt(key string, def ...int) int {
	m.record("GetInt", key, def)
	if m.GetIntFunc != nil {
		return m.GetIntFunc(key, def...)
	}
	return m.Get(key, def).(int)
}

func (m *MockCache) GetInt64(key string, def ...int64) int64 {
	m.record("GetInt64", key, def)
	if m.GetInt64Func != nil {
		return m.GetInt64Func(key, def...)
	}
	return m.Get(key, def).(int64)
}

func (m *MockCache) GetFloat64(key string, def ...float64) float64 {
	m.record("GetFloat64", key, def)
	if m.GetFloat64Func != nil {
		return m.GetFloat64Func(key, def...)
	}
	return m.Get(key, def).(float64)
}

func (m *MockCache) GetString(key string, def ...string) string {
	m.record("GetString", key, def)
	if m.GetStringFunc != nil {
		return m.GetStringFunc(key, def...)
	}
	return m.Get(key, def).(string)
}

func (m *MockCache) Has(key string) bool {
	m.record("Has", key)
	if m.HasFunc != nil {
		return m.HasFunc(key)
	}
	_, ok := m.Data[key]
	return ok
}

func (m *MockCache) Put(key string, value any, ttl time.Duration) error {
	m.record("Put", key, value, ttl)
	if m.PutFunc != nil {
		return m.PutFunc(key, value, ttl)
	}
	m.Data[key] = value
	return nil
}

func (m *MockCache) Forever(key string, value any) error {
	m.record("Forever", key, value)
	if m.ForeverFunc != nil {
		return m.ForeverFunc(key, value)
	}
	return m.Put(key, value, 0)
}

func (m *MockCache) Forget(key string) error {
	m.record("Forget", key)
	if m.ForgetFunc != nil {
		return m.ForgetFunc(key)
	}
	delete(m.Data, key)
	return nil
}

func (m *MockCache) Flush() error {
	m.record("Flush")
	if m.FlushFunc != nil {
		return m.FlushFunc()
	}
	m.Data = make(map[string]any)
	return nil
}

func (m *MockCache) Pull(key string, def ...any) any {
	m.record("Pull", key, def)
	if m.PullFunc != nil {
		return m.PullFunc(key, def...)
	}
	v := m.Get(key, def...)
	_ = m.Forget(key)
	return v
}

func (m *MockCache) Increment(key string, value ...int64) (int64, error) {
	m.record("Increment", key, value)
	if m.IncrementFunc != nil {
		return m.IncrementFunc(key, value...)
	}
	step := int64(1)
	if len(value) > 0 {
		step = value[0]
	}
	cur, _ := m.Get(key).(int64)
	cur += step
	m.Data[key] = cur
	return cur, nil
}

func (m *MockCache) Decrement(key string, value ...int64) (int64, error) {
	m.record("Decrement", key, value)
	if m.DecrementFunc != nil {
		return m.DecrementFunc(key, value...)
	}
	step := int64(1)
	if len(value) > 0 {
		step = value[0]
	}
	cur, _ := m.Get(key).(int64)
	cur -= step
	m.Data[key] = cur
	return cur, nil
}

func (m *MockCache) Remember(key string, ttl time.Duration, callback func() (any, error)) (any, error) {
	m.record("Remember", key, ttl)
	if m.RememberFunc != nil {
		return m.RememberFunc(key, ttl, callback)
	}
	if m.Has(key) {
		return m.Get(key), nil
	}
	v, err := callback()
	if err != nil {
		return nil, err
	}
	_ = m.Put(key, v, ttl)
	return v, nil
}

func (m *MockCache) RememberForever(key string, callback func() (any, error)) (any, error) {
	m.record("RememberForever", key)
	if m.RememberForeverFunc != nil {
		return m.RememberForeverFunc(key, callback)
	}
	return m.Remember(key, 0, callback)
}

func (m *MockCache) Many(keys []string) map[string]any {
	m.record("Many", keys)
	if m.ManyFunc != nil {
		return m.ManyFunc(keys)
	}
	res := make(map[string]any, len(keys))
	for _, k := range keys {
		res[k] = m.Get(k)
	}
	return res
}

func (m *MockCache) PutMany(values map[string]any, ttl time.Duration) error {
	m.record("PutMany", values, ttl)
	if m.PutManyFunc != nil {
		return m.PutManyFunc(values, ttl)
	}
	for k, v := range values {
		_ = m.Put(k, v, ttl)
	}
	return nil
}

func (m *MockCache) Tags(tags ...string) contracts.TaggedCache {
	m.record("Tags", tags)
	if m.TagsFunc != nil {
		return m.TagsFunc(tags...)
	}
	return m
}

func (m *MockCache) HGet(key, field string) (any, error) {
	m.record("HGet", key, field)
	if m.HGetFunc != nil {
		return m.HGetFunc(key, field)
	}
	h, _ := m.Data[key].(map[string]any)
	return h[field], nil
}

func (m *MockCache) HSet(key, field string, value any) error {
	m.record("HSet", key, field, value)
	if m.HSetFunc != nil {
		return m.HSetFunc(key, field, value)
	}
	h, _ := m.Data[key].(map[string]any)
	if h == nil {
		h = make(map[string]any)
		m.Data[key] = h
	}
	h[field] = value
	return nil
}

func (m *MockCache) HDel(key string, fields ...string) error {
	m.record("HDel", key, fields)
	if m.HDelFunc != nil {
		return m.HDelFunc(key, fields...)
	}
	h, _ := m.Data[key].(map[string]any)
	for _, f := range fields {
		delete(h, f)
	}
	return nil
}

func (m *MockCache) HExists(key, field string) bool {
	m.record("HExists", key, field)
	if m.HExistsFunc != nil {
		return m.HExistsFunc(key, field)
	}
	h, _ := m.Data[key].(map[string]any)
	_, ok := h[field]
	return ok
}

func (m *MockCache) HGetAll(key string) (map[string]any, error) {
	m.record("HGetAll", key)
	if m.HGetAllFunc != nil {
		return m.HGetAllFunc(key)
	}
	h, _ := m.Data[key].(map[string]any)
	return h, nil
}

func (m *MockCache) HLen(key string) int64 {
	m.record("HLen", key)
	if m.HLenFunc != nil {
		return m.HLenFunc(key)
	}
	h, _ := m.Data[key].(map[string]any)
	return int64(len(h))
}

func (m *MockCache) HKeys(key string) ([]string, error) {
	m.record("HKeys", key)
	if m.HKeysFunc != nil {
		return m.HKeysFunc(key)
	}
	h, _ := m.Data[key].(map[string]any)
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	return keys, nil
}

func (m *MockCache) Lock(key string, ttl time.Duration) contracts.CacheLock {
	m.record("Lock", key, ttl)
	if m.LockFunc != nil {
		return m.LockFunc(key, ttl)
	}
	return &mockCacheLock{}
}

func (m *MockCache) Store(name string) contracts.CacheStore {
	m.record("Store", name)
	if m.StoreFunc != nil {
		return m.StoreFunc(name)
	}
	return m
}

// 实现 TaggedCache 接口（与 CacheStore 复用）。
var _ contracts.TaggedCache = (*MockCache)(nil)
var _ contracts.Cache = (*MockCache)(nil)

type mockCacheLock struct{}

func (l *mockCacheLock) Acquire() bool               { return true }
func (l *mockCacheLock) Release() bool               { return true }
func (l *mockCacheLock) ForceRelease() bool          { return true }
func (l *mockCacheLock) Block(timeout time.Duration, callback ...func()) bool {
	if len(callback) > 0 {
		callback[0]()
	}
	return true
}
