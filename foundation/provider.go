package foundation

import "github.com/zhoudm1743/go-fast-framework/contracts"

// ServiceProvider 服务提供者接口。
// Register 阶段绑定服务到容器（不可使用其他服务）；
// Boot 阶段所有 Provider 已 Register 完成，可安全使用其他服务。
type ServiceProvider interface {
	// Register 将服务绑定到容器。此时其他服务可能尚未就绪，不可调用 MustMake。
	Register(app Application)
	// Boot 引导服务。所有 Provider 的 Register 均已执行完毕，可放心使用容器中的服务。
	Boot(app Application) error
}

// DeferredProvider 延迟服务提供者。
// 实现此接口的 Provider 在 Boot 阶段不会立即执行，而是等到
// 首次 Make 其声明的 key 时才自动触发 Register + Boot。
type DeferredProvider interface {
	ServiceProvider
	// DeferredServices 返回该 Provider 提供的服务 key 列表。
	DeferredServices() []string
}

// ── 可选扩展接口 ──────────────────────────────────────────────────────
//
// 插件/自定义 Provider 可按需实现以下可选接口。
// 框架在 Boot 过程的适当阶段自动检测并调用，无需在 Boot() 方法中手动处理。

// ConfigProvider 可选接口：声明插件的默认配置项。
//
// 框架在所有 Provider 的 Register 完成后、Boot 开始前，将返回的默认值
// 通过 contracts.Config.SetDefaults 写入配置服务（不覆盖用户已配置的值）。
//
// 使用示例：
//
//	func (sp *ServiceProvider) ConfigDefaults() map[string]any {
//	    return map[string]any{
//	        "redis.host":     "127.0.0.1",
//	        "redis.port":     6379,
//	        "redis.password": "",
//	        "redis.db":       0,
//	    }
//	}
type ConfigProvider interface {
	ConfigDefaults() map[string]any
}


// DBMigrator 可选接口：声明需要自动迁移的数据库模型。
//
// 框架在所有 Provider Boot 完成后、且 "db" 服务可用时，自动调用 MigrateDB。
//
// 使用示例：
//
//	func (sp *ServiceProvider) MigrateDB(db contracts.DB) error {
//	    return db.AutoMigrate(&Post{}, &Comment{})
//	}
type DBMigrator interface {
	MigrateDB(db contracts.DB) error
}

// RouteRegistrar 可选接口：声明插件提供的 HTTP 路由。
//
// 框架在所有 Provider Boot 完成后、且 "route" 服务可用时，自动调用 RegisterRoutes。
// 插件无需在 Boot() 中手动获取 Route 服务注册路由。
//
// 使用示例：
//
//	func (sp *ServiceProvider) RegisterRoutes(r contracts.Route) {
//	    r.Group("/admin", func(g contracts.Route) {
//	        g.Get("/stats", sp.handleStats)
//	    })
//	}
type RouteRegistrar interface {
	RegisterRoutes(r contracts.Route)
}
