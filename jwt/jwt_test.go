package jwt

import (
	"strings"
	"testing"

	gojwt "github.com/golang-jwt/jwt/v5"
	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// fakeConfig 实现 contracts.Config 用于测试。
type fakeConfig struct {
	values map[string]any
}

func newFakeConfig() *fakeConfig {
	return &fakeConfig{values: make(map[string]any)}
}

func (c *fakeConfig) set(key string, value any) *fakeConfig {
	c.values[key] = value
	return c
}

func (c *fakeConfig) GetString(key string, def ...string) string {
	d := ""
	if len(def) > 0 {
		d = def[0]
	}
	if v, ok := c.values[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return d
}

func (c *fakeConfig) GetInt(key string, def ...int) int {
	d := 0
	if len(def) > 0 {
		d = def[0]
	}
	if v, ok := c.values[key]; ok {
		if i, ok := v.(int); ok {
			return i
		}
	}
	return d
}

func (c *fakeConfig) GetBool(key string, def ...bool) bool {
	d := false
	if len(def) > 0 {
		d = def[0]
	}
	if v, ok := c.values[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return d
}

func (c *fakeConfig) GetFloat64(key string, def ...float64) float64 { return 0 }
func (c *fakeConfig) GetStringSlice(key string, def ...[]string) []string { return nil }
func (c *fakeConfig) GetStringMap(key string, def ...map[string]any) map[string]any {
	return nil
}

func (c *fakeConfig) Get(key string, def ...any) any {
	if v, ok := c.values[key]; ok {
		return v
	}
	if len(def) > 0 {
		return def[0]
	}
	return nil
}

func (c *fakeConfig) Set(key string, value any)          {}
func (c *fakeConfig) SetDefaults(defaults map[string]any)         {}
func (c *fakeConfig) Add(namespace string, config map[string]any) {}
func (c *fakeConfig) Env(key string, defaultValue ...any) any {
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return nil
}

var _ contracts.Config = (*fakeConfig)(nil)

// =============================================================================
// 默认 Guard 测试
// =============================================================================

func TestNew_DefaultGuard(t *testing.T) {
	cfg := newFakeConfig().
		set("jwt.secret", "test-secret").
		set("jwt.ttl", 60).
		set("jwt.alg", "HS256")

	j, err := New(cfg)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	if j == nil {
		t.Fatal("New() returned nil")
	}
}

func TestNew_MissingSecret(t *testing.T) {
	cfg := newFakeConfig()
	_, err := New(cfg)
	if err == nil {
		t.Fatal("New() expected error for missing jwt.secret")
	}
	if !strings.Contains(err.Error(), "jwt.secret") {
		t.Fatalf("expected error about jwt.secret, got: %v", err)
	}
}

func TestGenerateParse_RoundTrip(t *testing.T) {
	cfg := newFakeConfig().
		set("jwt.secret", "my-secret").
		set("jwt.ttl", 60)

	j, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	claims := gojwt.MapClaims{
		"user_id": "123",
		"role":    "admin",
	}

	token, err := j.GenerateToken(claims)
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}
	if token == "" {
		t.Fatal("GenerateToken() returned empty token")
	}

	parsed, err := j.ParseToken(token)
	if err != nil {
		t.Fatalf("ParseToken() error: %v", err)
	}
	if parsed["user_id"] != "123" {
		t.Fatalf("expected user_id=123, got %v", parsed["user_id"])
	}
	if parsed["role"] != "admin" {
		t.Fatalf("expected role=admin, got %v", parsed["role"])
	}
}

func TestRefreshToken(t *testing.T) {
	cfg := newFakeConfig().
		set("jwt.secret", "refresh-secret").
		set("jwt.ttl", 60)

	j, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	token, _ := j.GenerateToken(gojwt.MapClaims{"sub": "test"})
	newToken, err := j.RefreshToken(token)
	if err != nil {
		t.Fatalf("RefreshToken() error: %v", err)
	}
	if newToken == "" {
		t.Fatal("RefreshToken() returned empty token")
	}

	// 新 token 应能正常解析且保留原始 claims
	parsed, err := j.ParseToken(newToken)
	if err != nil {
		t.Fatalf("ParseToken() of refreshed token error: %v", err)
	}
	if parsed["sub"] != "test" {
		t.Fatalf("expected sub=test, got %v", parsed["sub"])
	}
}

// =============================================================================
// Guard 测试
// =============================================================================

func TestGuard_NamedGuard_RoundTrip(t *testing.T) {
	cfg := newFakeConfig().
		set("jwt.secret", "default-secret").
		set("jwt.guards.platform", map[string]any{}).
		set("jwt.guards.platform.secret", "platform-secret").
		set("jwt.guards.platform.ttl", 120).
		set("jwt.guards.platform.alg", "HS512")

	j, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	guard := j.Guard("platform")
	if guard == nil {
		t.Fatal("Guard() returned nil")
	}

	claims := gojwt.MapClaims{"app": "platform-v2"}
	token, err := guard.GenerateToken(claims)
	if err != nil {
		t.Fatalf("Guard.GenerateToken() error: %v", err)
	}

	parsed, err := guard.ParseToken(token)
	if err != nil {
		t.Fatalf("Guard.ParseToken() error: %v", err)
	}
	if parsed["app"] != "platform-v2" {
		t.Fatalf("expected app=platform-v2, got %v", parsed["app"])
	}
}

func TestGuard_CrossGuardIsolation(t *testing.T) {
	cfg := newFakeConfig().
		set("jwt.secret", "default-secret").
		set("jwt.guards.platform", map[string]any{}).
		set("jwt.guards.platform.secret", "platform-secret")

	j, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	// 用默认 guard 签发
	defaultToken, _ := j.GenerateToken(gojwt.MapClaims{"from": "default"})

	// 用 platform guard 解析默认 token — 应失败（密钥不同）
	platformGuard := j.Guard("platform")
	_, err = platformGuard.ParseToken(defaultToken)
	if err == nil {
		t.Fatal("expected error when parsing default token with platform guard")
	}
}

func TestGuard_EmptyName_ReturnsDefault(t *testing.T) {
	cfg := newFakeConfig().
		set("jwt.secret", "default-secret")

	j, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	g := j.Guard("")
	if g == nil {
		t.Fatal("Guard(\"\") returned nil")
	}

	// 空 name 的 guard 签发/解析结果应与默认一致
	token, _ := g.GenerateToken(gojwt.MapClaims{"key": "val"})
	parsed, err := j.ParseToken(token)
	if err != nil {
		t.Fatalf("default ParseToken of empty-guard token error: %v", err)
	}
	if parsed["key"] != "val" {
		t.Fatal("claims mismatch")
	}
}

func TestGuard_NotFound_Panics(t *testing.T) {
	cfg := newFakeConfig().
		set("jwt.secret", "default-secret")

	j, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for unknown guard")
		}
	}()

	j.Guard("nonexistent")
}

func TestGuard_Caching(t *testing.T) {
	cfg := newFakeConfig().
		set("jwt.secret", "default-secret").
		set("jwt.guards.admin", map[string]any{}).
		set("jwt.guards.admin.secret", "admin-secret")

	j, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	g1 := j.Guard("admin")
	g2 := j.Guard("admin")

	// 应返回同一实例（指针相等）
	if g1 != g2 {
		t.Fatal("Guard() should return cached instance")
	}
}
