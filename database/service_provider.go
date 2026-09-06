package database

import (
	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/foundation"
)

// ServiceProvider Database 服务提供者。
// 注意：本 Provider 仅注册 db 管理器，不内置任何 ORM 驱动。
// 业务需额外注册驱动插件，例如：
//
//	import gormdriver "github.com/zhoudm1743/gofast-gorm"
//	app.SetProviders(append(providers, &gormdriver.ServiceProvider{}))
//
// 或：
//
//	import xormdriver "github.com/zhoudm1743/gofast-xorm"
//	app.SetProviders(append(providers, &xormdriver.ServiceProvider{}))
type ServiceProvider struct{}

func (sp *ServiceProvider) Register(app foundation.Application) {
	// 注册新的 "db" 服务（contracts.DB）
	app.Singleton("db", func(app foundation.Application) (any, error) {
		cfg := app.MustMake("config").(contracts.Config)
		log := app.MustMake("log").(contracts.Log)
		return NewDBManager(cfg, log)
	})

}

func (sp *ServiceProvider) Boot(app foundation.Application) error {
	app.OnShutdown(func() {
		// 关闭新的 db 服务
		if db, err := app.Make("db"); err == nil {
			if closer, ok := db.(contracts.DB); ok {
				_ = closer.Close()
			}
		}
	})

	// 启用查询缓存插件（配置 database.cache.enabled: true）。
	// 底层使用框架 Cache 服务存储，仅显式调用 Query().Cache() 的查询生效。
	if cfg := app.MustMake("config").(contracts.Config); cfg.GetBool("database.cache.enabled", false) {
		if cache, err := app.Make("cache"); err == nil {
			if cm, ok := cache.(contracts.Cache); ok {
				if db, err := app.Make("db"); err == nil {
					if m, ok := db.(*dbManager); ok {
						if err := m.UseQueryCache(cm); err != nil {
							return err
						}
					}
				}
			}
		}
	}
	return nil
}
