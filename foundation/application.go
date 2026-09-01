package foundation

import (
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

const Version = "0.1.6"

// Application 应用实例接口，嵌入 Container。
type Application interface {
	Container

	// Boot 按声明顺序依次执行所有 Provider 的 Register，然后依次执行 Boot。
	// 多次调用安全（幂等），仅首次调用生效。
	Boot()
	// SetProviders 设置服务提供者列表（应在 Boot 之前调用）。
	SetProviders(providers []ServiceProvider)
	// BasePath 返回应用根目录；传入子路径时拼接返回。
	BasePath(path ...string) string
	// StoragePath 返回 storage 目录；传入子路径时拼接返回。
	StoragePath(path ...string) string
	// Version 返回框架版本号。
	Version() string
	// IsBooted 是否已引导（或正在引导中）。
	IsBooted() bool
	// Shutdown 优雅关闭，按注册逆序执行 shutdown hooks。
	Shutdown()
	// OnShutdown 注册关闭回调。
	OnShutdown(hook func())

	// ── 类型化服务快捷访问 ────────────────────────────────────────
	// 以下方法是对 MustMake + 类型断言的封装，专为插件开发者提供，
	// 避免在 Boot 中反复写 app.MustMake("config").(contracts.Config)。

	// Config 获取配置服务（等同于 MustMake("config").(contracts.Config)）。
	Config() contracts.Config
	// Log 获取日志服务（等同于 MustMake("log").(contracts.Log)）。
	Log() contracts.Log
	// Cache 获取缓存服务（等同于 MustMake("cache").(contracts.Cache)）。
	Cache() contracts.Cache
	// DB 获取数据库管理器（等同于 MustMake("db").(contracts.DB)）。
	DB() contracts.DB
	// Route 获取 HTTP 路由服务（等同于 MustMake("route").(contracts.Route)）。
	Route() contracts.Route
	// Storage 获取文件存储服务（等同于 MustMake("storage").(contracts.Storage)）。
	Storage() contracts.Storage
	// Fast 获取控制台服务（等同于 MustMake("fast").(contracts.Fast)）。
	Fast() contracts.Fast
	// Validator 获取验证服务（等同于 MustMake("validator").(contracts.Validation)）。
	Validator() contracts.Validation
	// Process 获取进程管理服务（等同于 MustMake("process").(contracts.Process)）。
	Process() contracts.Process
	// Mock 获取 Mock 管理服务（等同于 MustMake("mock").(contracts.MockManager)）。
	Mock() contracts.MockManager
	// Gate 获取授权服务（等同于 MustMake("gate").(contracts.Gate)）。
	Gate() contracts.Gate
}

// deferredEntry 将一个 DeferredProvider 与 sync.Once 绑定，保证线程安全地只初始化一次。
type deferredEntry struct {
	provider DeferredProvider
	once     sync.Once
	err      error
}

// application Application 接口的默认实现
type application struct {
	*container
	basePath      string
	providers     []ServiceProvider
	booted        atomic.Bool        // 原子操作保证并发安全的引导状态
	shutdownHooks []func()
	mu            sync.Mutex
	deferredMap   map[string]*deferredEntry // service key → deferred entry
}

// NewApplication 创建一个新的 Application 实例。
// basePath 为应用根目录（通常为 "."）。
func NewApplication(basePath string) Application {
	app := &application{
		basePath: basePath,
	}
	// 容器在构造时注入 app 引用，消除 setApp 竞态风险
	c := newContainer(app)
	app.container = c
	// 把 app 自己也注册到容器，方便 facades.App() 之外的场景使用
	app.Instance("app", app)
	return app
}

func (a *application) SetProviders(providers []ServiceProvider) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.providers = providers
}

// AppSetter 由 faades 包在 init() 中注入，Boot() 阶段自动调用。
// 避免 foundation 直接 import facades 产生循环依赖。
var AppSetter func(Application)

func (a *application) Boot() {
	// ── 幂等检查：使用 CAS 确保仅首次调用者执行引导 ──────────────────
	if !a.booted.CompareAndSwap(false, true) {
		return
	}

	if AppSetter != nil {
		AppSetter(a)
	}

	a.mu.Lock()
	a.deferredMap = make(map[string]*deferredEntry)

	// 分离即时 Provider 和延迟 Provider
	var immediate []ServiceProvider
	for _, p := range a.providers {
		if dp, ok := p.(DeferredProvider); ok {
			entry := &deferredEntry{provider: dp}
			for _, key := range dp.DeferredServices() {
				a.deferredMap[key] = entry
			}
		} else {
			immediate = append(immediate, p)
		}
	}

	// 收集所有 provider（含 deferred）的 ConfigDefaults
	var configProviders []ConfigProvider
	for _, p := range a.providers {
		if cp, ok := p.(ConfigProvider); ok {
			configProviders = append(configProviders, cp)
		}
	}

	// 释放锁：provider 的 Register/Boot 可能回调 Make、OnShutdown 等方法，
	// 若持锁调用会导致死锁。
	a.mu.Unlock()

	// Phase 1: Register 所有即时 Provider
	for _, p := range immediate {
		p.Register(a)
	}

	// Phase 1.5: 将各 ConfigProvider 声明的默认值写入 Config 服务
	// （包含 deferred provider 的默认配置，确保无论是否延迟加载，默认值都能生效）
	if len(configProviders) > 0 && a.Bound("config") {
		cfg := a.MustMake("config").(contracts.Config)
		for _, cp := range configProviders {
			cfg.SetDefaults(cp.ConfigDefaults())
		}
	}

	// Phase 2: Boot 所有即时 Provider
	for _, p := range immediate {
		if err := p.Boot(a); err != nil {
			panic(fmt.Sprintf("[GoFast] boot provider failed: %v", err))
		}
	}

	// Phase 3: 自动执行 DBMigrator（如果 db 服务可用）
	if a.Bound("db") {
		db := a.MustMake("db").(contracts.DB)
		for _, p := range immediate {
			if m, ok := p.(DBMigrator); ok {
				if err := m.MigrateDB(db); err != nil {
					panic(fmt.Sprintf("[GoFast] migrate (db) failed: %v", err))
				}
			}
		}
	}

	// Phase 4: 自动执行 RouteRegistrar（如果 route 服务可用）
	if a.Bound("route") {
		r := a.MustMake("route").(contracts.Route)
		for _, p := range immediate {
			if rr, ok := p.(RouteRegistrar); ok {
				rr.RegisterRoutes(r)
			}
		}
	}
}

// bootDeferredIfNeeded 在首次 Make 延迟服务时触发其 Provider 的 Register + Boot。
// 使用 sync.Once 确保同一 Provider 只初始化一次，并发安全。
// 初始化完成后自动检测并执行可选接口（Migrator/DBMigrator/RouteRegistrar）。
func (a *application) bootDeferredIfNeeded(key string) {
	a.mu.Lock()
	entry, ok := a.deferredMap[key]
	a.mu.Unlock()
	if !ok {
		return
	}

	entry.once.Do(func() {
		entry.provider.Register(a)
		entry.err = entry.provider.Boot(a)
		if entry.err != nil {
			return
		}
		// 延迟加载的 Provider 初始化完成后，执行可选的生命周期钩子
		a.runDeferredLifecycleHooks(entry.provider)
	})
	if entry.err != nil {
		panic(fmt.Sprintf("[GoFast] boot deferred provider failed: %v", entry.err))
	}
}

// runDeferredLifecycleHooks 对延迟初始化的 Provider 执行可选接口检测。
// 在 Provider 完成 Register+Boot 后由 bootDeferredIfNeeded 内部调用。
func (a *application) runDeferredLifecycleHooks(p ServiceProvider) {
	// DBMigrator（新 DB 接口）
	if a.Bound("db") {
		if m, ok := p.(DBMigrator); ok {
			db := a.MustMake("db").(contracts.DB)
			if err := m.MigrateDB(db); err != nil {
				panic(fmt.Sprintf("[GoFast] deferred migrate (db) failed: %v", err))
			}
		}
	}
	// RouteRegistrar
	if a.Bound("route") {
		if rr, ok := p.(RouteRegistrar); ok {
			r := a.MustMake("route").(contracts.Route)
			rr.RegisterRoutes(r)
		}
	}
}

// Make 解析服务。若 key 属于延迟 Provider，则先触发其初始化。
func (a *application) Make(key string) (any, error) {
	a.bootDeferredIfNeeded(key)
	return a.container.Make(key)
}

// MustMake 解析服务，失败时 panic。覆写以确保走 application.Make 的延迟逻辑。
func (a *application) MustMake(key string) any {
	v, err := a.Make(key)
	if err != nil {
		panic(err)
	}
	return v
}

// Bound 检查 key 是否已绑定或由延迟 Provider 声明。
func (a *application) Bound(key string) bool {
	a.mu.Lock()
	_, deferred := a.deferredMap[key]
	a.mu.Unlock()
	if deferred {
		return true
	}
	return a.container.Bound(key)
}

// Flush 清空所有绑定与延迟加载映射（测试用），并重置引导状态。
func (a *application) Flush() {
	a.mu.Lock()
	a.deferredMap = make(map[string]*deferredEntry)
	a.mu.Unlock()
	a.container.Flush()
	a.booted.Store(false)
}

func (a *application) BasePath(path ...string) string {
	if len(path) == 0 {
		return a.basePath
	}
	return filepath.Join(a.basePath, filepath.Join(path...))
}

func (a *application) StoragePath(path ...string) string {
	base := filepath.Join(a.basePath, "storage")
	if len(path) == 0 {
		return base
	}
	return filepath.Join(base, filepath.Join(path...))
}

func (a *application) Version() string {
	return Version
}

func (a *application) IsBooted() bool {
	return a.booted.Load()
}

func (a *application) OnShutdown(hook func()) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.shutdownHooks = append(a.shutdownHooks, hook)
}

func (a *application) Shutdown() {
	a.mu.Lock()
	hooks := make([]func(), len(a.shutdownHooks))
	copy(hooks, a.shutdownHooks)
	a.mu.Unlock()

	// 按注册逆序执行
	for i := len(hooks) - 1; i >= 0; i-- {
		hooks[i]()
	}
}

// ── 类型化服务快捷访问 ────────────────────────────────────────────────

func (a *application) Config() contracts.Config {
	return a.MustMake("config").(contracts.Config)
}

func (a *application) Log() contracts.Log {
	return a.MustMake("log").(contracts.Log)
}

func (a *application) Cache() contracts.Cache {
	return a.MustMake("cache").(contracts.Cache)
}


func (a *application) DB() contracts.DB {
	return a.MustMake("db").(contracts.DB)
}

func (a *application) Route() contracts.Route {
	return a.MustMake("route").(contracts.Route)
}

func (a *application) Storage() contracts.Storage {
	return a.MustMake("storage").(contracts.Storage)
}

func (a *application) Fast() contracts.Fast {
	return a.MustMake("fast").(contracts.Fast)
}

func (a *application) Validator() contracts.Validation {
	return a.MustMake("validator").(contracts.Validation)
}

func (a *application) Process() contracts.Process {
	return a.MustMake("process").(contracts.Process)
}

func (a *application) Mock() contracts.MockManager {
	return a.MustMake("mock").(contracts.MockManager)
}

func (a *application) Gate() contracts.Gate {
	return a.MustMake("gate").(contracts.Gate)
}
