package middleware

import (
	"strings"
	"testing"

	"github.com/zhoudm1743/go-fast-framework/facades"
	"github.com/zhoudm1743/go-fast-framework/foundation"
)

// mwTestConfig 中间件测试用 contracts.Config（内存 map + 点号路径）。
type mwTestConfig struct{ m map[string]any }

func (c *mwTestConfig) Env(key string, def ...any) any              { return c.Get(key, def...) }
func (c *mwTestConfig) Set(key string, value any)                   { c.m[key] = value }
func (c *mwTestConfig) SetDefaults(map[string]any)                  {}
func (c *mwTestConfig) Add(string, map[string]any)                  {}

func (c *mwTestConfig) lookup(key string) (any, bool) {
	if v, ok := c.m[key]; ok {
		return v, true
	}
	parts := strings.Split(key, ".")
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

func (c *mwTestConfig) Get(key string, def ...any) any {
	if v, ok := c.lookup(key); ok {
		return v
	}
	if len(def) > 0 {
		return def[0]
	}
	return nil
}
func (c *mwTestConfig) GetString(key string, def ...string) string {
	if v, ok := c.lookup(key); ok {
		s, _ := v.(string)
		return s
	}
	if len(def) > 0 {
		return def[0]
	}
	return ""
}
func (c *mwTestConfig) GetInt(key string, def ...int) int {
	if v, ok := c.lookup(key); ok {
		n, _ := v.(int)
		return n
	}
	if len(def) > 0 {
		return def[0]
	}
	return 0
}
func (c *mwTestConfig) GetBool(key string, def ...bool) bool {
	if v, ok := c.lookup(key); ok {
		b, _ := v.(bool)
		return b
	}
	if len(def) > 0 {
		return def[0]
	}
	return false
}
func (c *mwTestConfig) GetFloat64(key string, def ...float64) float64 {
	if v, ok := c.lookup(key); ok {
		f, _ := v.(float64)
		return f
	}
	if len(def) > 0 {
		return def[0]
	}
	return 0
}
func (c *mwTestConfig) GetStringSlice(key string, def ...[]string) []string {
	if v, ok := c.lookup(key); ok {
		ss, _ := v.([]string)
		return ss
	}
	if len(def) > 0 {
		return def[0]
	}
	return nil
}
func (c *mwTestConfig) GetStringMap(key string, def ...map[string]any) map[string]any {
	if v, ok := c.lookup(key); ok {
		sm, _ := v.(map[string]any)
		return sm
	}
	if len(def) > 0 {
		return def[0]
	}
	return nil
}

func setFacadeApp(t *testing.T, app foundation.Application) {
	t.Helper()
	facades.SetApp(app)
}
