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
	return nil
}
