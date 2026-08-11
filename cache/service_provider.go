package cache

import (
	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/foundation"
)

// ServiceProvider Cache 服务提供者。
type ServiceProvider struct{}

func (sp *ServiceProvider) Register(app foundation.Application) {
	app.Singleton("cache", func(app foundation.Application) (any, error) {
		cfg := app.MustMake("config").(contracts.Config)
		return NewCacheManager(cfg)
	})
}

func (sp *ServiceProvider) Boot(app foundation.Application) error {
	app.OnShutdown(func() {
		if c, err := app.Make("cache"); err == nil {
			if cm, ok := c.(*cacheManager); ok {
				cm.Stop()
			}
		}
	})
	return nil
}
