package jwt

import (
	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/foundation"
)

// ServiceProvider JWT 服务提供者。
type ServiceProvider struct{}

func (sp *ServiceProvider) Register(app foundation.Application) {
	app.Singleton("jwt", func(app foundation.Application) (any, error) {
		cfg := app.MustMake("config").(contracts.Config)
		return New(cfg)
	})
}

func (sp *ServiceProvider) Boot(_ foundation.Application) error {
	return nil
}
