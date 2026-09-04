package cors

import (
	"reflect"
	"testing"

	"github.com/zhoudm1743/go-fast-framework/config"
	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// newTestConfig 创建空配置实例并写入键值（路径不存在时 NewConfig 返回仅含默认值的空配置）。
func newTestConfig(t *testing.T, kv map[string]any) contracts.Config {
	t.Helper()
	cfg, err := config.NewConfig(t.TempDir() + "/none.yaml")
	if err != nil {
		t.Fatalf("创建配置失败: %v", err)
	}
	for k, v := range kv {
		cfg.Set(k, v)
	}
	return cfg
}

func TestLoadDefaults(t *testing.T) {
	opts := Load(newTestConfig(t, nil))

	if !opts.Wildcard() {
		t.Errorf("未配置时应放行所有源，实际 %v", opts.AllowOrigins)
	}
	if opts.AllowMethods != DefaultAllowMethods {
		t.Errorf("AllowMethods 默认值期望 %q，实际 %q", DefaultAllowMethods, opts.AllowMethods)
	}
	if opts.AllowHeaders != DefaultAllowHeaders {
		t.Errorf("AllowHeaders 默认值期望 %q，实际 %q", DefaultAllowHeaders, opts.AllowHeaders)
	}
	if opts.ExposeHeaders != "" {
		t.Errorf("ExposeHeaders 默认应为空，实际 %q", opts.ExposeHeaders)
	}
	// v0.7.12 行为：通配时不携带凭据
	if opts.AllowCredentials {
		t.Error("通配 + 未显式配置时不应允许凭据")
	}
	if opts.MaxAge != DefaultMaxAge {
		t.Errorf("MaxAge 默认值期望 %d，实际 %d", DefaultMaxAge, opts.MaxAge)
	}
}

func TestLoadOriginsFromString(t *testing.T) {
	opts := Load(newTestConfig(t, map[string]any{
		"server.cors_allow_origins": "https://a.com, https://b.com ,",
	}))
	want := []string{"https://a.com", "https://b.com"}
	if !reflect.DeepEqual(opts.AllowOrigins, want) {
		t.Errorf("AllowOrigins 期望 %v，实际 %v", want, opts.AllowOrigins)
	}
	if opts.Wildcard() {
		t.Error("配置了具体源后不应视为通配")
	}
}

func TestLoadOriginsFromArray(t *testing.T) {
	for name, raw := range map[string]any{
		"yaml数组":   []any{"https://a.com", "https://b.com"},
		"字符串数组": []string{"https://a.com", "https://b.com"},
	} {
		opts := Load(newTestConfig(t, map[string]any{"server.cors_allow_origins": raw}))
		want := []string{"https://a.com", "https://b.com"}
		if !reflect.DeepEqual(opts.AllowOrigins, want) {
			t.Errorf("%s: AllowOrigins 期望 %v，实际 %v", name, want, opts.AllowOrigins)
		}
	}
}

func TestLoadEmptyOriginsFallsBackToWildcard(t *testing.T) {
	// v0.7.12 行为：空数组视为未配置，保持通配
	opts := Load(newTestConfig(t, map[string]any{"server.cors_allow_origins": []any{}}))
	if !opts.Wildcard() {
		t.Errorf("空数组应回退为通配，实际 %v", opts.AllowOrigins)
	}
}

func TestLoadCredentialsAuto(t *testing.T) {
	// v0.7.12 行为：指定具体源时自动开启凭据
	opts := Load(newTestConfig(t, map[string]any{
		"server.cors_allow_origins": []any{"https://a.com"},
	}))
	if !opts.AllowCredentials {
		t.Error("指定具体源且未显式配置时，凭据应自动开启")
	}
}

func TestLoadCredentialsExplicit(t *testing.T) {
	for _, tc := range []struct {
		raw  any
		want bool
	}{
		{true, true},
		{false, false},
	} {
		opts := Load(newTestConfig(t, map[string]any{
			"server.cors_allow_origins":     []any{"https://a.com"},
			"server.cors_allow_credentials": tc.raw,
		}))
		if opts.AllowCredentials != tc.want {
			t.Errorf("cors_allow_credentials=%v 期望 %v，实际 %v", tc.raw, tc.want, opts.AllowCredentials)
		}
	}
}

func TestLoadMethodsHeadersExposeFromArrays(t *testing.T) {
	opts := Load(newTestConfig(t, map[string]any{
		"server.cors_allow_methods":  []any{"GET", "POST"},
		"server.cors_allow_headers":  []any{"X-Token", "X-Requested-With"},
		"server.cors_expose_headers": []any{"X-Total-Count", "X-Request-ID"},
	}))
	if opts.AllowMethods != "GET,POST" {
		t.Errorf("AllowMethods 期望 %q，实际 %q", "GET,POST", opts.AllowMethods)
	}
	if opts.AllowHeaders != "X-Token,X-Requested-With" {
		t.Errorf("AllowHeaders 期望 %q，实际 %q", "X-Token,X-Requested-With", opts.AllowHeaders)
	}
	if opts.ExposeHeaders != "X-Total-Count,X-Request-ID" {
		t.Errorf("ExposeHeaders 期望 %q，实际 %q", "X-Total-Count,X-Request-ID", opts.ExposeHeaders)
	}
}

func TestLoadMaxAge(t *testing.T) {
	opts := Load(newTestConfig(t, map[string]any{"server.cors_max_age": 3600}))
	if opts.MaxAge != 3600 {
		t.Errorf("MaxAge 期望 3600，实际 %d", opts.MaxAge)
	}
	// <=0 表示不输出 Access-Control-Max-Age
	opts = Load(newTestConfig(t, map[string]any{"server.cors_max_age": 0}))
	if opts.MaxAge != 0 {
		t.Errorf("MaxAge=0 时期望保持 0，实际 %d", opts.MaxAge)
	}
}

func TestHSTSMaxAge(t *testing.T) {
	if got := HSTSMaxAge(newTestConfig(t, nil)); got != 31536000 {
		t.Errorf("HSTSMaxAge 默认值期望 31536000，实际 %d", got)
	}
	if got := HSTSMaxAge(newTestConfig(t, map[string]any{"server.security_hsts_max_age": 86400})); got != 86400 {
		t.Errorf("HSTSMaxAge 期望 86400，实际 %d", got)
	}
}

func TestMatchOrigin(t *testing.T) {
	wildcard := Options{}
	if v, ok := wildcard.MatchOrigin("https://a.com"); !ok || v != "*" {
		t.Errorf("通配应命中并返回 *，实际 %q %v", v, ok)
	}
	if _, ok := wildcard.MatchOrigin(""); ok {
		t.Error("空 Origin 不应命中")
	}

	specific := Options{AllowOrigins: []string{"https://a.com", "https://B.com"}}
	if v, ok := specific.MatchOrigin("https://a.com"); !ok || v != "https://a.com" {
		t.Errorf("命中时应回显请求 Origin，实际 %q %v", v, ok)
	}
	// 源比较不区分大小写（scheme/host 本身不区分大小写）
	if _, ok := specific.MatchOrigin("https://b.com"); !ok {
		t.Error("Origin 大小写不同但主机相同时应命中")
	}
	if _, ok := specific.MatchOrigin("https://evil.com"); ok {
		t.Error("未列入白名单的 Origin 不应命中")
	}
}
