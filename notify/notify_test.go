package notify

import (
	"strings"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// testConfig 是测试用的 contracts.Config 实现（内存 map，支持点号路径）。
type testConfig struct {
	m map[string]any
}

func newTestConfig(m map[string]any) *testConfig {
	return &testConfig{m: m}
}

func (c *testConfig) Env(key string, def ...any) any              { return c.Get(key, def...) }
func (c *testConfig) Set(key string, value any)                   { c.m[key] = value }
func (c *testConfig) SetDefaults(defaults map[string]any)         {}
func (c *testConfig) Add(namespace string, config map[string]any) {}

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
func (c *testConfig) Get(key string, def ...any) any {
	if v, ok := c.lookup(key); ok {
		return v
	}
	if len(def) > 0 {
		return def[0]
	}
	return nil
}

// lookup 按点号路径查找；支持嵌套 map 与扁平字面 key 两种形态。
func (c *testConfig) lookup(key string) (any, bool) {
	if v, ok := c.m[key]; ok {
		return v, true
	}
	parts := strings.Split(key, ".")
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

var _ contracts.Config = (*testConfig)(nil)
