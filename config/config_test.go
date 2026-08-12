package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// 辅助函数：创建临时 YAML 配置文件并返回路径。
func tempConfigFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("写入临时配置文件失败: %v", err)
	}
	return path
}

// ── NewConfig 测试 ────────────────────────────────────────────────────

func TestNewConfig_Valid(t *testing.T) {
	path := tempConfigFile(t, `
server:
  port: 8080
  host: "localhost"
  debug: true
  rate: 0.75
`)
	cfg, err := NewConfig(path)
	if err != nil {
		t.Fatalf("NewConfig 应该成功: %v", err)
	}
	if cfg == nil {
		t.Fatal("NewConfig 不应返回 nil")
	}
}

func TestNewConfig_MissingFile_Optional(t *testing.T) {
	cfg, err := NewConfig("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("NewConfig 对不存在的 YAML 应可选跳过，不应报错: %v", err)
	}
	if cfg == nil {
		t.Fatal("缺失 YAML 时仍应返回可用 Config 实例")
	}
	// 仅依赖 Go 默认值 / 运行时 Set
	cfg.Add("app", map[string]any{"name": "GoFast"})
	if v := cfg.GetString("app.name"); v != "GoFast" {
		t.Fatalf("期望 GoFast，得到 %v", v)
	}
}

func TestNewConfig_InvalidYAML_StillErrors(t *testing.T) {
	path := tempConfigFile(t, "server:\n  port: [unclosed")
	cfg, err := NewConfig(path)
	if err == nil {
		t.Fatal("存在但无法解析的 YAML 应返回错误")
	}
	if cfg != nil {
		t.Fatal("解析失败时应返回 nil config")
	}
}

func TestNewConfig_ReturnsInterface(t *testing.T) {
	path := tempConfigFile(t, "key: value")
	cfg, err := NewConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	// 编译时已验证返回接口类型，运行时再确认
	_ = cfg
}

// ── Get 测试 ──────────────────────────────────────────────────────────

func TestGet_ExistingKey(t *testing.T) {
	path := tempConfigFile(t, "server:\n  port: 8080")
	cfg, _ := NewConfig(path)
	v := cfg.Get("server.port")
	if v != 8080 {
		t.Fatalf("期望 8080，得到 %v", v)
	}
}

func TestGet_MissingKey_NoDefault(t *testing.T) {
	path := tempConfigFile(t, "server:\n  port: 8080")
	cfg, _ := NewConfig(path)
	v := cfg.Get("nonexistent")
	if v != nil {
		t.Fatalf("不存在的 key 应返回 nil，得到 %v", v)
	}
}

func TestGet_MissingKey_WithDefault(t *testing.T) {
	path := tempConfigFile(t, "server:\n  port: 8080")
	cfg, _ := NewConfig(path)
	v := cfg.Get("nonexistent", "fallback")
	if v != "fallback" {
		t.Fatalf("期望 fallback，得到 %v", v)
	}
}

func TestGet_NestedKey(t *testing.T) {
	path := tempConfigFile(t, "database:\n  postgres:\n    host: pg.example.com")
	cfg, _ := NewConfig(path)
	v := cfg.Get("database.postgres.host")
	if v != "pg.example.com" {
		t.Fatalf("期望 pg.example.com，得到 %v", v)
	}
}

// ── GetString 测试 ────────────────────────────────────────────────────

func TestGetString_ExistingKey(t *testing.T) {
	path := tempConfigFile(t, "app:\n  name: go-fast")
	cfg, _ := NewConfig(path)
	v := cfg.GetString("app.name")
	if v != "go-fast" {
		t.Fatalf("期望 go-fast，得到 %v", v)
	}
}

func TestGetString_WithDefault(t *testing.T) {
	path := tempConfigFile(t, "app:\n  name: go-fast")
	cfg, _ := NewConfig(path)
	v := cfg.GetString("app.unknown", "default-app")
	if v != "default-app" {
		t.Fatalf("期望 default-app，得到 %v", v)
	}
}

// ── GetInt 测试 ───────────────────────────────────────────────────────

func TestGetInt_ExistingKey(t *testing.T) {
	path := tempConfigFile(t, "server:\n  port: 9090\n  workers: 4")
	cfg, _ := NewConfig(path)
	if v := cfg.GetInt("server.port"); v != 9090 {
		t.Fatalf("期望 9090，得到 %v", v)
	}
	if v := cfg.GetInt("server.workers"); v != 4 {
		t.Fatalf("期望 4，得到 %v", v)
	}
}

func TestGetInt_WithDefault(t *testing.T) {
	path := tempConfigFile(t, "server:\n  port: 9090")
	cfg, _ := NewConfig(path)
	v := cfg.GetInt("server.max_conns", 100)
	if v != 100 {
		t.Fatalf("期望 100，得到 %v", v)
	}
}

func TestGetInt_MissingKey_NoDefault(t *testing.T) {
	path := tempConfigFile(t, "server:\n  port: 9090")
	cfg, _ := NewConfig(path)
	v := cfg.GetInt("nonexistent")
	if v != 0 {
		t.Fatalf("不存在的 int key 应返回 0，得到 %v", v)
	}
}

// ── GetBool 测试 ──────────────────────────────────────────────────────

func TestGetBool_ExistingKey(t *testing.T) {
	path := tempConfigFile(t, "server:\n  debug: true\n  tls: false")
	cfg, _ := NewConfig(path)
	if v := cfg.GetBool("server.debug"); !v {
		t.Fatal("期望 true")
	}
	if v := cfg.GetBool("server.tls"); v {
		t.Fatal("期望 false")
	}
}

func TestGetBool_WithDefault(t *testing.T) {
	path := tempConfigFile(t, "server:\n  debug: true")
	cfg, _ := NewConfig(path)
	if v := cfg.GetBool("server.cors", true); !v {
		t.Fatal("默认值应返回 true")
	}
	if v := cfg.GetBool("server.cors", false); v {
		t.Fatal("默认值应返回 false")
	}
}

// ── GetFloat64 测试 ───────────────────────────────────────────────────

func TestGetFloat64_ExistingKey(t *testing.T) {
	path := tempConfigFile(t, "threshold: 0.85")
	cfg, _ := NewConfig(path)
	v := cfg.GetFloat64("threshold")
	if v != 0.85 {
		t.Fatalf("期望 0.85，得到 %v", v)
	}
}

func TestGetFloat64_WithDefault(t *testing.T) {
	path := tempConfigFile(t, "threshold: 0.85")
	cfg, _ := NewConfig(path)
	v := cfg.GetFloat64("ratio", 0.5)
	if v != 0.5 {
		t.Fatalf("期望 0.5，得到 %v", v)
	}
}

// ── GetStringSlice 测试 ───────────────────────────────────────────────

func TestGetStringSlice_ExistingKey(t *testing.T) {
	path := tempConfigFile(t, "cors:\n  origins:\n    - http://a.com\n    - http://b.com")
	cfg, _ := NewConfig(path)
	v := cfg.GetStringSlice("cors.origins")
	if len(v) != 2 || v[0] != "http://a.com" || v[1] != "http://b.com" {
		t.Fatalf("期望 [http://a.com http://b.com]，得到 %v", v)
	}
}

func TestGetStringSlice_WithDefault(t *testing.T) {
	path := tempConfigFile(t, "server:\n  port: 8080")
	cfg, _ := NewConfig(path)
	d := []string{"a", "b"}
	v := cfg.GetStringSlice("nonexistent", d)
	if len(v) != 2 || v[0] != "a" || v[1] != "b" {
		t.Fatalf("期望 [a b]，得到 %v", v)
	}
}

func TestGetStringSlice_MissingKey_NoDefault(t *testing.T) {
	path := tempConfigFile(t, "server:\n  port: 8080")
	cfg, _ := NewConfig(path)
	v := cfg.GetStringSlice("nonexistent")
	if v != nil {
		t.Fatalf("不存在的 slice key 应返回 nil，得到 %v", v)
	}
}

// ── GetStringMap 测试 ─────────────────────────────────────────────────

func TestGetStringMap_ExistingKey(t *testing.T) {
	path := tempConfigFile(t, "redis:\n  host: 127.0.0.1\n  port: 6379")
	cfg, _ := NewConfig(path)
	v := cfg.GetStringMap("redis")
	if v["host"] != "127.0.0.1" || v["port"] != 6379 {
		t.Fatalf("期望 map[host:127.0.0.1 port:6379]，得到 %v", v)
	}
}

func TestGetStringMap_WithDefault(t *testing.T) {
	path := tempConfigFile(t, "server:\n  port: 8080")
	cfg, _ := NewConfig(path)
	d := map[string]any{"host": "default", "port": 0}
	v := cfg.GetStringMap("nonexistent", d)
	if v["host"] != "default" || v["port"] != 0 {
		t.Fatalf("期望默认值 map[host:default port:0]，得到 %v", v)
	}
}

func TestGetStringMap_MissingKey_NoDefault(t *testing.T) {
	path := tempConfigFile(t, "server:\n  port: 8080")
	cfg, _ := NewConfig(path)
	v := cfg.GetStringMap("nonexistent")
	if v != nil {
		t.Fatalf("不存在的 map key 应返回 nil，得到 %v", v)
	}
}

func TestGetStringMap_MapStringString_Set(t *testing.T) {
	path := tempConfigFile(t, "")
	cfg, _ := NewConfig(path)
	cfg.Set("headers", map[string]string{"X-Request-Id": "abc", "Accept": "json"})
	v := cfg.GetStringMap("headers")
	if v["X-Request-Id"] != "abc" || v["Accept"] != "json" {
		t.Fatalf("Set map[string]string 后 GetStringMap 应可读，得到 %v", v)
	}
}

func TestGetStringMap_MapStringString_Add(t *testing.T) {
	resetPendingAdds()
	defer resetPendingAdds()

	path := tempConfigFile(t, "")
	cfg, _ := NewConfig(path)
	cfg.Add("database", map[string]any{
		"connections": map[string]string{"main": "sqlite", "slave": "mysql"},
	})
	v := cfg.GetStringMap("database.connections")
	if v["main"] != "sqlite" || v["slave"] != "mysql" {
		t.Fatalf("Add 嵌套 map[string]string 后 GetStringMap 应可读，得到 %v", v)
	}
}

func TestGetStringMap_MapStringInt_Set(t *testing.T) {
	path := tempConfigFile(t, "")
	cfg, _ := NewConfig(path)
	cfg.Set("limits", map[string]int{"max": 100, "min": 1})
	v := cfg.GetStringMap("limits")
	if v["max"] != 100 || v["min"] != 1 {
		t.Fatalf("Set map[string]int 后 GetStringMap 应可读，得到 %v", v)
	}
}

func TestGetStringMap_ScalarReturnsNil(t *testing.T) {
	path := tempConfigFile(t, "")
	cfg, _ := NewConfig(path)
	cfg.Set("notamap", "hello")
	if v := cfg.GetStringMap("notamap"); v != nil {
		t.Fatalf("标量值应返回 nil，得到 %v", v)
	}
}

func TestGetStringMap_EmptyMap(t *testing.T) {
	path := tempConfigFile(t, "")
	cfg, _ := NewConfig(path)
	cfg.Set("empty", map[string]any{})
	v := cfg.GetStringMap("empty")
	if v == nil {
		t.Fatal("空 map 应返回非 nil 的空 map")
	}
	if len(v) != 0 {
		t.Fatalf("期望空 map，得到 %v", v)
	}
}

func TestAdd_MapStringInt(t *testing.T) {
	resetPendingAdds()
	defer resetPendingAdds()

	path := tempConfigFile(t, "")
	cfg, _ := NewConfig(path)
	cfg.Add("app", map[string]any{
		"limits": map[string]int{"max": 100, "min": 1},
	})
	if v := cfg.GetInt("app.limits.max"); v != 100 {
		t.Fatalf("期望 100，得到 %v", v)
	}
	if v := cfg.GetInt("app.limits.min"); v != 1 {
		t.Fatalf("期望 1，得到 %v", v)
	}
}

func TestAdd_MapStringBool(t *testing.T) {
	resetPendingAdds()
	defer resetPendingAdds()

	path := tempConfigFile(t, "")
	cfg, _ := NewConfig(path)
	cfg.Add("flags", map[string]any{
		"switches": map[string]bool{"debug": true, "trace": false},
	})
	if v := cfg.GetBool("flags.switches.debug"); !v {
		t.Fatal("期望 true")
	}
	if v := cfg.GetBool("flags.switches.trace"); v {
		t.Fatal("期望 false")
	}
}

func TestAdd_NestedTypedMaps(t *testing.T) {
	resetPendingAdds()
	defer resetPendingAdds()

	path := tempConfigFile(t, "")
	cfg, _ := NewConfig(path)
	cfg.Add("db", map[string]any{
		"connections": map[string]any{
			"main": map[string]string{"driver": "gorm", "engine": "sqlite"},
		},
	})
	if v := cfg.GetString("db.connections.main.driver"); v != "gorm" {
		t.Fatalf("期望 gorm，得到 %v", v)
	}
	if v := cfg.GetString("db.connections.main.engine"); v != "sqlite" {
		t.Fatalf("期望 sqlite，得到 %v", v)
	}
}

func TestGetRegistry_DeepCopy(t *testing.T) {
	resetPendingAdds()
	defer resetPendingAdds()

	original := map[string]any{
		"name": "GoFast",
		"pool": map[string]any{"size": 10},
	}
	Add("app", original)

	registry := GetRegistry()
	registry["app"]["name"] = "mutated"
	if sub, ok := registry["app"]["pool"].(map[string]any); ok {
		sub["size"] = 999
	}

	registry2 := GetRegistry()
	if registry2["app"]["name"] != "GoFast" {
		t.Fatalf("外层 map 应隔离，期望 GoFast，得到 %v", registry2["app"]["name"])
	}
	pool, ok := registry2["app"]["pool"].(map[string]any)
	if !ok || pool["size"] != 10 {
		t.Fatalf("嵌套 map 应隔离，得到 %v", registry2["app"]["pool"])
	}

	original["name"] = "changed-in-source"
	registry3 := GetRegistry()
	if registry3["app"]["name"] != "GoFast" {
		t.Fatalf("源 map 变更不应影响注册表，得到 %v", registry3["app"]["name"])
	}
}

// ── Env 测试 ──────────────────────────────────────────────────────────

func TestEnv_ExistingVar(t *testing.T) {
	os.Setenv("GOFAST_TEST_ENV_VAR", "test_value")
	defer os.Unsetenv("GOFAST_TEST_ENV_VAR")

	path := tempConfigFile(t, "key: value")
	cfg, _ := NewConfig(path)
	v := cfg.Env("GOFAST_TEST_ENV_VAR")
	if v != "test_value" {
		t.Fatalf("期望 test_value，得到 %v", v)
	}
}

func TestEnv_MissingVar_NoDefault(t *testing.T) {
	path := tempConfigFile(t, "key: value")
	cfg, _ := NewConfig(path)
	v := cfg.Env("NONEXISTENT_ENV_VAR_FOR_TEST")
	if v != nil {
		t.Fatalf("不存在且无默认值应返回 nil，得到 %v", v)
	}
}

func TestEnv_MissingVar_WithDefault(t *testing.T) {
	path := tempConfigFile(t, "key: value")
	cfg, _ := NewConfig(path)
	v := cfg.Env("NONEXISTENT_ENV_VAR_FOR_TEST", "fallback")
	if v != "fallback" {
		t.Fatalf("期望 fallback，得到 %v", v)
	}
}

func TestEnv_EmptyVar_ReturnsDefault(t *testing.T) {
	os.Setenv("GOFAST_EMPTY_VAR", "")
	defer os.Unsetenv("GOFAST_EMPTY_VAR")

	path := tempConfigFile(t, "key: value")
	cfg, _ := NewConfig(path)
	v := cfg.Env("GOFAST_EMPTY_VAR", "fallback")
	// 空字符串被视为未设置，应返回默认值
	if v != "fallback" {
		t.Fatalf("空字符串环境变量应返回默认值 fallback，得到 %v", v)
	}
}

// ── Set + Get 往返测试 ────────────────────────────────────────────────

func TestSet_Get_RoundTrip(t *testing.T) {
	path := tempConfigFile(t, "key: original")
	cfg, _ := NewConfig(path)

	cfg.Set("key", "modified")
	v := cfg.GetString("key")
	if v != "modified" {
		t.Fatalf("Set 后 GetString 应返回新值，期望 modified，得到 %v", v)
	}
}

func TestSet_NewKey(t *testing.T) {
	path := tempConfigFile(t, "existing: value")
	cfg, _ := NewConfig(path)

	cfg.Set("new.key", 42)
	v := cfg.GetInt("new.key")
	if v != 42 {
		t.Fatalf("期望 42，得到 %v", v)
	}
}

func TestSet_OverridesDefault(t *testing.T) {
	path := tempConfigFile(t, "")
	cfg, _ := NewConfig(path)

	cfg.SetDefaults(map[string]any{"theme": "light"})
	cfg.Set("theme", "dark")
	v := cfg.GetString("theme")
	if v != "dark" {
		t.Fatalf("Set 应覆盖 SetDefaults，期望 dark，得到 %v", v)
	}
}

// ── SetDefaults 测试 ──────────────────────────────────────────────────

func TestSetDefaults_ProvidesFallback(t *testing.T) {
	path := tempConfigFile(t, "")
	cfg, _ := NewConfig(path)

	cfg.SetDefaults(map[string]any{"app.debug": false, "app.name": "go-fast"})
	if cfg.GetBool("app.debug") {
		t.Fatal("期望 false")
	}
	if cfg.GetString("app.name") != "go-fast" {
		t.Fatalf("期望 go-fast，得到 %v", cfg.GetString("app.name"))
	}
}

func TestSetDefaults_DoesNotOverrideConfig(t *testing.T) {
	path := tempConfigFile(t, "app:\n  name: my-app")
	cfg, _ := NewConfig(path)

	cfg.SetDefaults(map[string]any{"app.name": "go-fast"})
	// 配置文件中的值优先级高于 SetDefaults
	if cfg.GetString("app.name") != "my-app" {
		t.Fatalf("SetDefaults 不应覆盖配置文件值，期望 my-app，得到 %v", cfg.GetString("app.name"))
	}
}

func TestSetDefaults_DoesNotOverrideSet(t *testing.T) {
	path := tempConfigFile(t, "")
	cfg, _ := NewConfig(path)

	cfg.Set("app.name", "set-value")
	cfg.SetDefaults(map[string]any{"app.name": "default-value"})
	if cfg.GetString("app.name") != "set-value" {
		t.Fatalf("SetDefaults 不应覆盖 Set 的值，期望 set-value，得到 %v", cfg.GetString("app.name"))
	}
}

// ── 并发安全测试 ──────────────────────────────────────────────────────

func TestConfig_ConcurrentRead(t *testing.T) {
	path := tempConfigFile(t, "key: value")
	cfg, _ := NewConfig(path)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = cfg.GetString("key")
			_ = cfg.GetInt("key")
			_ = cfg.GetBool("key")
			_ = cfg.GetFloat64("key")
			_ = cfg.GetStringSlice("key")
			_ = cfg.GetStringMap("key")
		}()
	}
	wg.Wait()
}

func TestConfig_ConcurrentReadWrite(t *testing.T) {
	path := tempConfigFile(t, "counter: 0")
	cfg, _ := NewConfig(path)

	var wg sync.WaitGroup
	// 写 goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			cfg.Set("counter", i)
		}
	}()

	// 读 goroutine
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = cfg.GetInt("counter")
			_ = cfg.GetString("counter")
		}()
	}
	wg.Wait()
}

func TestConfig_ConcurrentSetDefaults(t *testing.T) {
	path := tempConfigFile(t, "")
	cfg, _ := NewConfig(path)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			cfg.SetDefaults(map[string]any{"thread": idx})
			_ = cfg.GetInt("thread")
		}(i)
	}
	wg.Wait()
}

// ── Env 与 Get 隔离测试 ───────────────────────────────────────────────

func TestEnv_DoesNotAffectGet(t *testing.T) {
	os.Setenv("GOFAST_TEST_ISOLATION", "from_env")
	defer os.Unsetenv("GOFAST_TEST_ISOLATION")

	path := tempConfigFile(t, "gofast_test_isolation: from_config")
	cfg, _ := NewConfig(path)

	// Env 读取环境变量（精确匹配 key 名）
	envVal := cfg.Env("GOFAST_TEST_ISOLATION")
	if envVal != "from_env" {
		t.Fatalf("Env 应返回环境变量值 from_env，得到 %v", envVal)
	}

	// Get 只读配置文件，不受环境变量影响（已移除 AutomaticEnv）
	cfgVal := cfg.GetString("gofast_test_isolation")
	if cfgVal != "from_config" {
		t.Fatalf("Get 应返回配置文件值 from_config，得到 %v", cfgVal)
	}
}

// ── 空配置文件测试 ────────────────────────────────────────────────────

func TestEmptyConfig(t *testing.T) {
	path := tempConfigFile(t, "")
	cfg, err := NewConfig(path)
	if err != nil {
		t.Fatalf("空配置文件应正常加载: %v", err)
	}
	// 所有 Get 都应返回默认值
	if v := cfg.GetString("any.key", "default"); v != "default" {
		t.Fatalf("期望 default，得到 %v", v)
	}
}

// ── 数组/YAML 列表值测试 ──────────────────────────────────────────────

func TestGet_YAMLList(t *testing.T) {
	path := tempConfigFile(t, "hosts:\n  - 10.0.0.1\n  - 10.0.0.2\n  - 10.0.0.3")
	cfg, _ := NewConfig(path)
	v := cfg.Get("hosts")
	if v == nil {
		t.Fatal("列表类型的配置不应为 nil")
	}
}

// ── Add（Go 配置文件注册）测试 ──────────────────────────────────────

// resetPendingAdds 清空全局缓冲区，用于测试隔离。
func resetPendingAdds() {
	addMu.Lock()
	defer addMu.Unlock()
	pendingAdds = nil
}

func TestAdd_And_Get(t *testing.T) {
	resetPendingAdds()
	defer resetPendingAdds()

	path := tempConfigFile(t, "")
	cfg, _ := NewConfig(path)

	cfg.Add("app", map[string]any{
		"name":  "GoFast",
		"debug": false,
		"port":  8080,
	})

	if v := cfg.GetString("app.name"); v != "GoFast" {
		t.Fatalf("期望 GoFast，得到 %v", v)
	}
	if v := cfg.GetBool("app.debug"); v {
		t.Fatal("期望 false")
	}
	if v := cfg.GetInt("app.port"); v != 8080 {
		t.Fatalf("期望 8080，得到 %v", v)
	}
}

func TestAdd_NestedMap(t *testing.T) {
	resetPendingAdds()
	defer resetPendingAdds()

	path := tempConfigFile(t, "")
	cfg, _ := NewConfig(path)

	cfg.Add("database", map[string]any{
		"default": "sqlite",
		"pool": map[string]any{
			"max_idle_conns": 10,
			"max_open_conns": 100,
		},
	})

	if v := cfg.GetString("database.default"); v != "sqlite" {
		t.Fatalf("期望 sqlite，得到 %v", v)
	}
	if v := cfg.GetInt("database.pool.max_idle_conns"); v != 10 {
		t.Fatalf("期望 10，得到 %v", v)
	}
	if v := cfg.GetInt("database.pool.max_open_conns"); v != 100 {
		t.Fatalf("期望 100，得到 %v", v)
	}
}

func TestAdd_YAMLWins(t *testing.T) {
	resetPendingAdds()
	defer resetPendingAdds()

	path := tempConfigFile(t, "app:\n  name: from-yaml")
	cfg, _ := NewConfig(path)

	cfg.Add("app", map[string]any{
		"name":  "from-go",
		"debug": false,
	})

	// YAML 值应覆盖 Go 默认值
	if v := cfg.GetString("app.name"); v != "from-yaml" {
		t.Fatalf("YAML 应覆盖 Go 默认值，期望 from-yaml，得到 %v", v)
	}
	// YAML 中不存在的键仍使用 Go 默认值
	if v := cfg.GetBool("app.debug"); v {
		t.Fatal("期望 false")
	}
}

func TestAdd_SetWins(t *testing.T) {
	resetPendingAdds()
	defer resetPendingAdds()

	path := tempConfigFile(t, "")
	cfg, _ := NewConfig(path)

	cfg.Add("app", map[string]any{"name": "go-default"})
	cfg.Set("app.name", "runtime")

	if v := cfg.GetString("app.name"); v != "runtime" {
		t.Fatalf("Set 应覆盖 Add 默认值，期望 runtime，得到 %v", v)
	}
}

func TestAdd_SameNS_Merge(t *testing.T) {
	resetPendingAdds()
	defer resetPendingAdds()

	path := tempConfigFile(t, "")
	cfg, _ := NewConfig(path)

	// 第一次 Add
	cfg.Add("app", map[string]any{"name": "GoFast", "debug": true})
	// 第二次 Add：同命名空间应逐键合并，而非整体替换
	cfg.Add("app", map[string]any{"debug": false, "env": "production"})

	if v := cfg.GetString("app.name"); v != "GoFast" {
		t.Fatalf("第一次 Add 的 name 不应丢失，期望 GoFast，得到 %v", v)
	}
	if v := cfg.GetBool("app.debug"); v {
		t.Fatal("第二次 Add 应覆盖 debug 为 false")
	}
	if v := cfg.GetString("app.env"); v != "production" {
		t.Fatalf("期望 production，得到 %v", v)
	}
}

func TestAdd_Concurrent(t *testing.T) {
	resetPendingAdds()
	defer resetPendingAdds()

	path := tempConfigFile(t, "")
	cfg, _ := NewConfig(path)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			cfg.Add("ns", map[string]any{"key": idx})
			_ = cfg.GetInt("ns.key")
		}(i)
	}
	wg.Wait()
	// 无竞态即通过
}

func TestApplyPendingAdds(t *testing.T) {
	resetPendingAdds()
	defer resetPendingAdds()

	// 模拟 config/ 包 init() 阶段的注册
	Add("app", map[string]any{"name": "GoFast", "debug": false})

	path := tempConfigFile(t, "server:\n  port: 3000")
	cfg, _ := NewConfig(path)
	applyPendingAdds(cfg)

	if v := cfg.GetString("app.name"); v != "GoFast" {
		t.Fatalf("期望 GoFast，得到 %v", v)
	}
	if v := cfg.GetInt("server.port"); v != 3000 {
		t.Fatalf("YAML 值应保留，期望 3000，得到 %v", v)
	}
}

func TestApplyPendingAdds_YAMLWins(t *testing.T) {
	resetPendingAdds()
	defer resetPendingAdds()

	Add("app", map[string]any{"name": "from-go"})

	path := tempConfigFile(t, "app:\n  name: from-yaml")
	cfg, _ := NewConfig(path)
	applyPendingAdds(cfg)

	if v := cfg.GetString("app.name"); v != "from-yaml" {
		t.Fatalf("YAML 应覆盖 Go 注册表值，期望 from-yaml，得到 %v", v)
	}
}

func TestPackageAdd_Concurrent(t *testing.T) {
	resetPendingAdds()
	defer resetPendingAdds()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			Add("ns", map[string]any{"key": idx})
			GetRegistry()
		}(i)
	}
	wg.Wait()

	registry := GetRegistry()
	if len(registry) == 0 {
		t.Fatal("注册表不应为空")
	}
}
