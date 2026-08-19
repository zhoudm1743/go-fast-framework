package database

import (
	"github.com/zhoudm1743/go-fast-framework/contracts"
	gormdriver "github.com/zhoudm1743/go-fast-framework/database/drivers/gormdriver"
	"github.com/zhoudm1743/go-fast-framework/foundation"
)

// ServiceProvider Database 服务提供者。
type ServiceProvider struct{}

func (sp *ServiceProvider) Register(app foundation.Application) {
	// 内置注册 GORM 驱动工厂
	RegisterDriver("gormdriver", func(cfg ConnectionConfig, log contracts.Log) (contracts.Driver, error) {
		return gormdriver.NewGormDriver(cfg, log)
	})

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
