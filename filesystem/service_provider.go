package filesystem

import (
	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/foundation"
)

// ServiceProvider Filesystem 服务提供者。
type ServiceProvider struct{}

func (sp *ServiceProvider) Register(app foundation.Application) {
	app.Singleton("storage", func(app foundation.Application) (any, error) {
		cfg := app.MustMake("config").(contracts.Config)
		return NewStorage(cfg)
	})
}

func (sp *ServiceProvider) Boot(app foundation.Application) error {
	return nil
}
