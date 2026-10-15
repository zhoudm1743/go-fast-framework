package auth

import (
	"fmt"
	"sync"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/foundation"
)

// manager 实现 contracts.Auth：守卫调度 + 驱动/提供者注册表。
type manager struct {
	app       foundation.Application
	factories map[string]contracts.GuardFactory
	providers map[string]contracts.UserProvider
	guards    map[string]contracts.Guard // 已构建的守卫缓存
	mu        sync.Mutex
}

// NewManager 创建认证管理器，内置注册 jwt 守卫驱动。
func NewManager(app foundation.Application) contracts.Auth {
	m := &manager{
		app:       app,
		factories: map[string]contracts.GuardFactory{},
		providers: map[string]contracts.UserProvider{},
		guards:    map[string]contracts.Guard{},
	}
	m.factories["jwt"] = jwtGuardFactory(app)
	return m
}

func (m *manager) Guard(name ...string) contracts.Guard {
	guardName := m.app.Config().GetString("auth.default_guard", "api")
	if len(name) > 0 && name[0] != "" {
		guardName = name[0]
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if g, ok := m.guards[guardName]; ok {
		return g
	}

	gcfg := m.app.Config().GetStringMap("auth.guards." + guardName)
	if gcfg == nil {
		panic(fmt.Sprintf("auth: 守卫 %q 未在 auth.guards 中声明", guardName))
	}
	driverName, _ := gcfg["driver"].(string)
	if driverName == "" {
		driverName = "jwt"
	}
	providerName, _ := gcfg["provider"].(string)
	provider, ok := m.providers[providerName]
	if !ok {
		panic(fmt.Sprintf("auth: 守卫 %q 引用的用户提供者 %q 未注册", guardName, providerName))
	}
	factory, ok := m.factories[driverName]
	if !ok {
		panic(fmt.Sprintf("auth: 守卫 %q 引用的驱动 %q 未注册", guardName, driverName))
	}
	g, err := factory(guardName, gcfg, provider)
	if err != nil {
		panic(fmt.Sprintf("auth: 构建守卫 %q 失败: %v", guardName, err))
	}
	m.guards[guardName] = g
	return g
}

func (m *manager) Extend(name string, factory contracts.GuardFactory) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.factories[name]; exists {
		return fmt.Errorf("auth: 守卫驱动 %q 已注册", name)
	}
	m.factories[name] = factory
	return nil
}

func (m *manager) Provider(name string, p contracts.UserProvider) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.providers[name]; exists {
		return fmt.Errorf("auth: 用户提供者 %q 已注册", name)
	}
	m.providers[name] = p
	return nil
}

// jwtGuardFactory 内置 jwt 守卫驱动：token 管道委托 contracts.JWT。
// jwt_guard 映射规则：guard 配置的 jwt_guard 键 > jwt.guards.<guard 名> 存在 > 默认 jwt。
func jwtGuardFactory(app foundation.Application) contracts.GuardFactory {
	return func(name string, gcfg map[string]any, provider contracts.UserProvider) (contracts.Guard, error) {
		jwtSvc := app.MustMake("jwt").(contracts.JWT)

		var driver contracts.JWTDriver = jwtSvc
		jwtGuardName := name
		if v, ok := gcfg["jwt_guard"].(string); ok && v != "" {
			jwtGuardName = v
		}
		if app.Config().Get("jwt.guards."+jwtGuardName) != nil {
			driver = jwtSvc.Guard(jwtGuardName)
		}

		var cache contracts.CacheStore
		if app.Bound("cache") {
			cache = app.Cache()
		}
		return newJWTGuard(name, driver, provider, cache), nil
	}
}
