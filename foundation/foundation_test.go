package foundation

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// ── Container 测试 ──────────────────────────────────────────────────

func TestContainer_Bind(t *testing.T) {
	app := NewApplication(".")
	callCount := 0
	app.Bind("svc", func(a Application) (any, error) {
		callCount++
		return callCount, nil
	})

	v1, _ := app.Make("svc")
	v2, _ := app.Make("svc")
	if v1.(int) != 1 || v2.(int) != 2 {
		t.Fatalf("Bind should create new instance each time, got %v %v", v1, v2)
	}
}

func TestContainer_Singleton(t *testing.T) {
	app := NewApplication(".")
	callCount := 0
	app.Singleton("svc", func(a Application) (any, error) {
		callCount++
		return "instance", nil
	})

	v1, _ := app.Make("svc")
	v2, _ := app.Make("svc")
	if v1 != v2 {
		t.Fatal("Singleton should return same instance")
	}
	if callCount != 1 {
		t.Fatalf("Singleton factory should be called once, got %d", callCount)
	}
}

func TestContainer_Instance(t *testing.T) {
	app := NewApplication(".")
	obj := &struct{ Name string }{Name: "test"}
	app.Instance("svc", obj)

	v, err := app.Make("svc")
	if err != nil {
		t.Fatal(err)
	}
	if v != obj {
		t.Fatal("Instance should return exact same pointer")
	}
}

func TestContainer_MustMake_Panic(t *testing.T) {
	app := NewApplication(".")
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("MustMake should panic for unknown key")
		}
	}()
	app.MustMake("nonexistent")
}

func TestContainer_Bound(t *testing.T) {
	app := NewApplication(".")
	if app.Bound("svc") {
		t.Fatal("should not be bound")
	}
	app.Instance("svc", 1)
	if !app.Bound("svc") {
		t.Fatal("should be bound")
	}
}

func TestContainer_Flush(t *testing.T) {
	app := NewApplication(".")
	app.Instance("svc", 1)
	app.Flush()
	if app.Bound("svc") {
		t.Fatal("Flush should clear all bindings")
	}
}

func TestContainer_Singleton_ConcurrentSafe(t *testing.T) {
	app := NewApplication(".")
	var count int64
	app.Singleton("svc", func(a Application) (any, error) {
		atomic.AddInt64(&count, 1)
		return "ok", nil
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v := app.MustMake("svc")
			if v != "ok" {
				t.Errorf("unexpected value: %v", v)
			}
		}()
	}
	wg.Wait()

	if atomic.LoadInt64(&count) != 1 {
		t.Fatalf("singleton factory called %d times, expected 1", count)
	}
}

// ── Application 测试 ────────────────────────────────────────────────

type testProvider struct {
	registerOrder *[]string
	bootOrder     *[]string
	name          string
}

func (p *testProvider) Register(app Application) {
	*p.registerOrder = append(*p.registerOrder, p.name)
}

func (p *testProvider) Boot(app Application) error {
	*p.bootOrder = append(*p.bootOrder, p.name)
	return nil
}

func TestApplication_Boot_Order(t *testing.T) {
	var regOrder, bootOrder []string

	providers := []ServiceProvider{
		&testProvider{registerOrder: &regOrder, bootOrder: &bootOrder, name: "config"},
		&testProvider{registerOrder: &regOrder, bootOrder: &bootOrder, name: "log"},
		&testProvider{registerOrder: &regOrder, bootOrder: &bootOrder, name: "db"},
	}

	app := NewApplication(".")
	app.SetProviders(providers)
	app.Boot()

	// Register 全部先于 Boot
	expectedReg := []string{"config", "log", "db"}
	expectedBoot := []string{"config", "log", "db"}

	for i, v := range expectedReg {
		if regOrder[i] != v {
			t.Fatalf("Register order mismatch at %d: got %s, want %s", i, regOrder[i], v)
		}
	}
	for i, v := range expectedBoot {
		if bootOrder[i] != v {
			t.Fatalf("Boot order mismatch at %d: got %s, want %s", i, bootOrder[i], v)
		}
	}
}

func TestApplication_Boot_Idempotent(t *testing.T) {
	count := 0
	p := &countProvider{count: &count}
	app := NewApplication(".")
	app.SetProviders([]ServiceProvider{p})
	app.Boot()
	app.Boot() // 重复调用
	if count != 1 {
		t.Fatalf("Boot should be idempotent, register called %d times", count)
	}
}

type countProvider struct {
	count *int
}

func (p *countProvider) Register(app Application)   { *p.count++ }
func (p *countProvider) Boot(app Application) error { return nil }

func TestApplication_Shutdown_ReverseOrder(t *testing.T) {
	var order []string
	app := NewApplication(".")
	app.OnShutdown(func() { order = append(order, "first") })
	app.OnShutdown(func() { order = append(order, "second") })
	app.OnShutdown(func() { order = append(order, "third") })
	app.Shutdown()

	expected := []string{"third", "second", "first"}
	for i, v := range expected {
		if order[i] != v {
			t.Fatalf("Shutdown order mismatch at %d: got %s, want %s", i, order[i], v)
		}
	}
}

func TestApplication_BasePath(t *testing.T) {
	app := NewApplication("/app")
	if app.BasePath() != "/app" {
		t.Fatalf("unexpected: %s", app.BasePath())
	}
	got := app.BasePath("config.yaml")
	// filepath.Join 在不同 OS 下分隔符不同，只检查包含关系
	if got != "/app/config.yaml" && got != "\\app\\config.yaml" && got != "/app\\config.yaml" {
		t.Fatalf("unexpected: %s", got)
	}
}

func TestApplication_Version(t *testing.T) {
	app := NewApplication(".")
	if app.Version() == "" {
		t.Fatal("version should not be empty")
	}
}

// ── DeferredProvider 测试 ───────────────────────────────────────────

// deferredTestProvider 模拟延迟服务提供者
type deferredTestProvider struct {
	registered bool
	booted     bool
	keys       []string
	value      any
}

func (p *deferredTestProvider) Register(app Application) {
	p.registered = true
	for _, key := range p.keys {
		val := p.value
		app.Singleton(key, func(a Application) (any, error) {
			return val, nil
		})
	}
}

func (p *deferredTestProvider) Boot(app Application) error {
	p.booted = true
	return nil
}

func (p *deferredTestProvider) DeferredServices() []string {
	return p.keys
}

func TestDeferredProvider_NotBootedDuringBoot(t *testing.T) {
	dp := &deferredTestProvider{keys: []string{"lazy"}, value: "hello"}
	app := NewApplication(".")
	app.SetProviders([]ServiceProvider{dp})
	app.Boot()

	if dp.registered {
		t.Fatal("deferred provider should NOT be registered during Boot")
	}
	if dp.booted {
		t.Fatal("deferred provider should NOT be booted during Boot")
	}
}

func TestDeferredProvider_BootedOnFirstMake(t *testing.T) {
	dp := &deferredTestProvider{keys: []string{"lazy"}, value: "hello"}
	app := NewApplication(".")
	app.SetProviders([]ServiceProvider{dp})
	app.Boot()

	v, err := app.Make("lazy")
	if err != nil {
		t.Fatalf("Make failed: %v", err)
	}
	if v != "hello" {
		t.Fatalf("expected 'hello', got %v", v)
	}
	if !dp.registered {
		t.Fatal("deferred provider should be registered after Make")
	}
	if !dp.booted {
		t.Fatal("deferred provider should be booted after Make")
	}
}

func TestDeferredProvider_MustMakeTriggersDeferred(t *testing.T) {
	dp := &deferredTestProvider{keys: []string{"lazy"}, value: 42}
	app := NewApplication(".")
	app.SetProviders([]ServiceProvider{dp})
	app.Boot()

	v := app.MustMake("lazy")
	if v != 42 {
		t.Fatalf("expected 42, got %v", v)
	}
	if !dp.registered || !dp.booted {
		t.Fatal("deferred provider should be fully initialized after MustMake")
	}
}

func TestDeferredProvider_MultipleKeys(t *testing.T) {
	dp := &deferredTestProvider{keys: []string{"svcA", "svcB"}, value: "shared"}
	app := NewApplication(".")
	app.SetProviders([]ServiceProvider{dp})
	app.Boot()

	// 通过第一个 key 触发
	v1, _ := app.Make("svcA")
	if v1 != "shared" {
		t.Fatalf("expected 'shared', got %v", v1)
	}

	// 第二个 key 也应该可用（同一 Provider 只初始化一次）
	v2, _ := app.Make("svcB")
	if v2 != "shared" {
		t.Fatalf("expected 'shared', got %v", v2)
	}
}

func TestDeferredProvider_Bound(t *testing.T) {
	dp := &deferredTestProvider{keys: []string{"lazy"}, value: "x"}
	app := NewApplication(".")
	app.SetProviders([]ServiceProvider{dp})
	app.Boot()

	// 未 Make 前也应返回 true
	if !app.Bound("lazy") {
		t.Fatal("Bound should return true for deferred key before Make")
	}

	// Make 后依然 true
	app.MustMake("lazy")
	if !app.Bound("lazy") {
		t.Fatal("Bound should return true for deferred key after Make")
	}
}

func TestDeferredProvider_OnlyBootedOnce(t *testing.T) {
	var regCount, bootCount int
	dp := &onceCountDeferredProvider{
		keys:      []string{"once"},
		regCount:  &regCount,
		bootCount: &bootCount,
	}
	app := NewApplication(".")
	app.SetProviders([]ServiceProvider{dp})
	app.Boot()

	app.MustMake("once")
	app.MustMake("once")
	app.MustMake("once")

	if regCount != 1 {
		t.Fatalf("Register called %d times, expected 1", regCount)
	}
	if bootCount != 1 {
		t.Fatalf("Boot called %d times, expected 1", bootCount)
	}
}

type onceCountDeferredProvider struct {
	keys      []string
	regCount  *int
	bootCount *int
}

func (p *onceCountDeferredProvider) Register(app Application) {
	*p.regCount++
	for _, key := range p.keys {
		app.Singleton(key, func(a Application) (any, error) {
			return "ok", nil
		})
	}
}
func (p *onceCountDeferredProvider) Boot(app Application) error {
	*p.bootCount++
	return nil
}
func (p *onceCountDeferredProvider) DeferredServices() []string {
	return p.keys
}

func TestDeferredProvider_MixedWithImmediate(t *testing.T) {
	var regOrder []string

	immediate := &testProvider{registerOrder: &regOrder, bootOrder: &regOrder, name: "config"}
	deferred := &deferredTestProvider{keys: []string{"lazy"}, value: "deferred_val"}

	app := NewApplication(".")
	app.SetProviders([]ServiceProvider{immediate, deferred})
	app.Boot()

	// immediate 已注册，deferred 未注册
	if len(regOrder) != 2 { // Register + Boot for config
		t.Fatalf("expected 2 order entries (reg+boot for config), got %d", len(regOrder))
	}
	if deferred.registered {
		t.Fatal("deferred should not be registered yet")
	}

	// 触发 deferred
	v := app.MustMake("lazy")
	if v != "deferred_val" {
		t.Fatalf("expected 'deferred_val', got %v", v)
	}
	if !deferred.registered || !deferred.booted {
		t.Fatal("deferred should be fully initialized after MustMake")
	}
}

func TestDeferredProvider_ConcurrentMake(t *testing.T) {
	var regCount int64
	dp := &atomicCountDeferredProvider{
		keys:     []string{"concurrent"},
		regCount: &regCount,
	}
	app := NewApplication(".")
	app.SetProviders([]ServiceProvider{dp})
	app.Boot()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v := app.MustMake("concurrent")
			if v != "ok" {
				t.Errorf("unexpected value: %v", v)
			}
		}()
	}
	wg.Wait()

	if atomic.LoadInt64(&regCount) != 1 {
		t.Fatalf("Register called %d times, expected 1", atomic.LoadInt64(&regCount))
	}
}

type atomicCountDeferredProvider struct {
	keys     []string
	regCount *int64
}

func (p *atomicCountDeferredProvider) Register(app Application) {
	atomic.AddInt64(p.regCount, 1)
	for _, key := range p.keys {
		app.Singleton(key, func(a Application) (any, error) {
			return "ok", nil
		})
	}
}
func (p *atomicCountDeferredProvider) Boot(app Application) error { return nil }
func (p *atomicCountDeferredProvider) DeferredServices() []string { return p.keys }

// ── Flush 清理 deferredMap 测试 ──────────────────────────────────────

func TestFlush_ClearsDeferredMap(t *testing.T) {
	dp := &deferredTestProvider{keys: []string{"lazy"}, value: "hello"}
	app := NewApplication(".")
	app.SetProviders([]ServiceProvider{dp})
	app.Boot()

	// 触发延迟加载
	app.MustMake("lazy")

	// Flush 清空
	app.Flush()

	// Flush 后 Bound 应返回 false
	if app.Bound("lazy") {
		t.Fatal("Flush should clear deferred map, Bound should return false")
	}

	// Flush 后 IsBooted 应返回 false
	if app.IsBooted() {
		t.Fatal("Flush should reset booted flag")
	}
}

// ── Singleton 失败重试测试 ──────────────────────────────────────────

func TestSingleton_RetryAfterError(t *testing.T) {
	app := NewApplication(".")
	attempts := 0

	app.Singleton("fallible", func(a Application) (any, error) {
		attempts++
		if attempts < 3 {
			return nil, fmt.Errorf("temporary error")
		}
		return "recovered", nil
	})

	// 前两次应该失败
	for i := 0; i < 2; i++ {
		_, err := app.Make("fallible")
		if err == nil {
			t.Fatalf("attempt %d should fail", i+1)
		}
	}

	// 第三次应该成功（共 3 次调用，第 3 次成功）
	v, err := app.Make("fallible")
	if err != nil {
		t.Fatalf("expected success on retry, got: %v", err)
	}
	if v != "recovered" {
		t.Fatalf("expected 'recovered', got %v", v)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}

	// 后续应该返回缓存的成功结果，不再调用 factory
	v2, err := app.Make("fallible")
	if err != nil {
		t.Fatalf("expected cached success, got: %v", err)
	}
	if v2 != "recovered" {
		t.Fatalf("expected 'recovered', got %v", v2)
	}
	if attempts != 3 {
		t.Fatalf("factory should not be called again, attempts=%d", attempts)
	}
}

func TestSingleton_ErrorNotCached(t *testing.T) {
	app := NewApplication(".")
	failCount := 0

	app.Singleton("svc", func(a Application) (any, error) {
		failCount++
		if failCount == 1 {
			return nil, fmt.Errorf("first attempt fails")
		}
		return "ok", nil
	})

	// 第一次失败
	_, err := app.Make("svc")
	if err == nil {
		t.Fatal("expected error")
	}

	// 第二次成功（重试）
	v, err := app.Make("svc")
	if err != nil {
		t.Fatalf("expected success on retry: %v", err)
	}
	if v != "ok" {
		t.Fatalf("expected 'ok', got %v", v)
	}
	if failCount != 2 {
		t.Fatalf("expected 2 attempts, got %d", failCount)
	}
}

// ── DeferredProvider ConfigDefaults 测试 ─────────────────────────────

type deferredConfigProvider struct {
	registered bool
	booted     bool
}

func (p *deferredConfigProvider) Register(app Application) {
	p.registered = true
	app.Singleton("deferred_svc", func(a Application) (any, error) {
		return "deferred", nil
	})
}

func (p *deferredConfigProvider) Boot(app Application) error {
	p.booted = true
	return nil
}

func (p *deferredConfigProvider) DeferredServices() []string {
	return []string{"deferred_svc"}
}

func (p *deferredConfigProvider) ConfigDefaults() map[string]any {
	return map[string]any{
		"deferred.key": "default_value",
	}
}

func TestDeferredProvider_ConfigDefaults(t *testing.T) {
	dp := &deferredConfigProvider{}
	app := NewApplication(".")

	// 注册一个假的 config 服务来接收 ConfigDefaults
	configValues := make(map[string]any)
	app.Singleton("config", func(a Application) (any, error) {
		return &fakeConfig{values: configValues}, nil
	})

	app.SetProviders([]ServiceProvider{dp})
	app.Boot()

	// 即使 deferred provider 尚未初始化，ConfigDefaults 也应在 Boot 阶段生效
	if configValues["deferred.key"] != "default_value" {
		t.Fatalf("ConfigDefaults should be applied during Boot, got: %v", configValues)
	}
}

type fakeConfig struct{ values map[string]any }

func (f *fakeConfig) Env(key string, defaultValue ...any) any                      { return nil }
func (f *fakeConfig) Get(key string, defaultValue ...any) any                      { return nil }
func (f *fakeConfig) GetString(key string, defaultValue ...string) string          { return "" }
func (f *fakeConfig) GetInt(key string, defaultValue ...int) int                   { return 0 }
func (f *fakeConfig) GetBool(key string, defaultValue ...bool) bool                { return false }
func (f *fakeConfig) GetFloat64(key string, defaultValue ...float64) float64       { return 0 }
func (f *fakeConfig) GetStringSlice(key string, defaultValue ...[]string) []string { return nil }
func (f *fakeConfig) GetStringMap(key string, defaultValue ...map[string]any) map[string]any {
	return nil
}
func (f *fakeConfig) Set(key string, value any) {}
func (f *fakeConfig) SetDefaults(defaults map[string]any) {
	for k, v := range defaults {
		f.values[k] = v
	}
}
func (f *fakeConfig) Add(namespace string, config map[string]any) {}

// ── Boot 并发安全测试 ────────────────────────────────────────────────

func TestBoot_ConcurrentSafety(t *testing.T) {
	count := 0
	p := &countProvider{count: &count}
	app := NewApplication(".")
	app.SetProviders([]ServiceProvider{p})

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			app.Boot()
		}()
	}
	wg.Wait()

	// CAS 保证仅执行一次
	if count != 1 {
		t.Fatalf("Boot should execute exactly once, got %d", count)
	}
	if !app.IsBooted() {
		t.Fatal("IsBooted should return true after concurrent Boot calls")
	}
}

// ── Application 类型化快捷方法测试 ────────────────────────────────────

func TestApplication_Fast(t *testing.T) {
	app := NewApplication(".")
	app.Singleton("fast", func(a Application) (any, error) {
		return &dummyFast{}, nil
	})
	app.Boot()

	fast := app.Fast()
	if fast == nil {
		t.Fatal("Fast() should return non-nil")
	}
}

func TestApplication_Validator(t *testing.T) {
	app := NewApplication(".")
	app.Singleton("validator", func(a Application) (any, error) {
		return &dummyValidation{}, nil
	})
	app.Boot()

	v := app.Validator()
	if v == nil {
		t.Fatal("Validator() should return non-nil")
	}
}

type dummyFast struct{}

func (d *dummyFast) Register(commands []contracts.ConsoleCommand) {}
func (d *dummyFast) Run(args []string) error                      { return nil }
func (d *dummyFast) Call(command string) error                    { return nil }
func (d *dummyFast) RunSync(args []string) error                  { return nil }
func (d *dummyFast) CallSync(command string) error                { return nil }
func (d *dummyFast) RunAsync(args []string)                       {}
func (d *dummyFast) CallAsync(command string)                     {}

type dummyValidation struct{}

func (d *dummyValidation) Validate(obj any) error                           { return nil }
func (d *dummyValidation) RegisterRule(rule contracts.ValidationRule) error { return nil }
