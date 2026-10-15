package auth

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/mock"
)

// ── 测试辅助 ────────────────────────────────────────────────────────

// testConfig 测试用 contracts.Config（内存 map，扁平 key 与点号路径均支持）。
type testConfig struct {
	m map[string]any
}

func newTestConfig(m map[string]any) *testConfig { return &testConfig{m: m} }

func (c *testConfig) Env(key string, def ...any) any              { return c.Get(key, def...) }
func (c *testConfig) Set(key string, value any)                   { c.m[key] = value }
func (c *testConfig) SetDefaults(defaults map[string]any)         {}
func (c *testConfig) Add(namespace string, config map[string]any) {}

func (c *testConfig) lookup(key string) (any, bool) {
	if v, ok := c.m[key]; ok {
		return v, true
	}
	parts := strings.Split(key, ".")
	// 支持扁平 key 与嵌套 map 混合：逐级缩短前缀匹配扁平 key，再向嵌套 map 内走
	for i := len(parts) - 1; i > 0; i-- {
		if v, ok := c.m[strings.Join(parts[:i], ".")]; ok {
			cur := v
			for _, p := range parts[i:] {
				mm, ok := cur.(map[string]any)
				if !ok {
					return nil, false
				}
				cur, ok = mm[p]
				if !ok {
					return nil, false
				}
			}
			return cur, true
		}
	}
	// 纯嵌套 map 路径
	var cur any = c.m
	for _, p := range parts {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = mm[p]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func (c *testConfig) Get(key string, def ...any) any {
	if v, ok := c.lookup(key); ok {
		return v
	}
	if len(def) > 0 {
		return def[0]
	}
	return nil
}
func (c *testConfig) GetString(key string, def ...string) string {
	if v, ok := c.lookup(key); ok {
		s, _ := v.(string)
		return s
	}
	if len(def) > 0 {
		return def[0]
	}
	return ""
}
func (c *testConfig) GetInt(key string, def ...int) int {
	if v, ok := c.lookup(key); ok {
		n, _ := v.(int)
		return n
	}
	if len(def) > 0 {
		return def[0]
	}
	return 0
}
func (c *testConfig) GetBool(key string, def ...bool) bool {
	if v, ok := c.lookup(key); ok {
		b, _ := v.(bool)
		return b
	}
	if len(def) > 0 {
		return def[0]
	}
	return false
}
func (c *testConfig) GetFloat64(key string, def ...float64) float64 {
	if v, ok := c.lookup(key); ok {
		f, _ := v.(float64)
		return f
	}
	if len(def) > 0 {
		return def[0]
	}
	return 0
}
func (c *testConfig) GetStringSlice(key string, def ...[]string) []string {
	if v, ok := c.lookup(key); ok {
		ss, _ := v.([]string)
		return ss
	}
	if len(def) > 0 {
		return def[0]
	}
	return nil
}
func (c *testConfig) GetStringMap(key string, def ...map[string]any) map[string]any {
	if v, ok := c.lookup(key); ok {
		sm, _ := v.(map[string]any)
		return sm
	}
	if len(def) > 0 {
		return def[0]
	}
	return nil
}

// stubCtx 最小 http Context：仅实现 Header/Value/WithValue，其余方法调用会 panic。
type stubCtx struct {
	contracts.Context
	headers map[string]string
	values  map[string]any
}

func newStubCtx() *stubCtx {
	return &stubCtx{headers: map[string]string{}, values: map[string]any{}}
}

func (c *stubCtx) Header(key string) string { return c.headers[key] }

func (c *stubCtx) Value(key string) any { return c.values[key] }

func (c *stubCtx) WithValue(key string, value any) contracts.Context {
	c.values[key] = value
	return c
}

// testUser 测试用 AuthUser。
type testUser struct {
	ID       int64
	Name     string
	Password string // bcrypt hash
}

func (u *testUser) GetAuthID() any         { return u.ID }
func (u *testUser) GetAuthPassword() string { return u.Password }

// stubProvider 测试用 UserProvider。
type stubProvider struct {
	mu             sync.Mutex
	byIDFunc       func(ctx contracts.Context, id any) (contracts.AuthUser, error)
	byIdentityFunc func(ctx contracts.Context, identity string) (contracts.AuthUser, error)
	byIDCalls      []any
}

func (p *stubProvider) RetrieveByID(ctx contracts.Context, id any) (contracts.AuthUser, error) {
	p.mu.Lock()
	p.byIDCalls = append(p.byIDCalls, id)
	p.mu.Unlock()
	if p.byIDFunc != nil {
		return p.byIDFunc(ctx, id)
	}
	return nil, ErrUserNotFound
}

func (p *stubProvider) RetrieveByIdentity(ctx contracts.Context, identity string) (contracts.AuthUser, error) {
	if p.byIdentityFunc != nil {
		return p.byIdentityFunc(ctx, identity)
	}
	return nil, ErrUserNotFound
}

// newStubCache 基于 MockCache 构造内存缓存。
func newStubCache() *mock.MockCache {
	store := map[string]any{}
	return &mock.MockCache{
		HasFunc: func(key string) bool {
			_, ok := store[key]
			return ok
		},
		PutFunc: func(key string, value any, ttl time.Duration) error {
			store[key] = value
			return nil
		},
		ForeverFunc: func(key string, value any) error {
			store[key] = value
			return nil
		},
		ForgetFunc: func(key string) error {
			delete(store, key)
			return nil
		},
	}
}

func assertErrIs(t *testing.T, err, target error) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error %v, got nil", target)
	}
	if !errors.Is(err, target) {
		t.Fatalf("expected error %v, got %v", target, err)
	}
}
