package ratelimit

import (
	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/foundation"
)

// ServiceProvider Ratelimit 服务提供者。
type ServiceProvider struct{}

func (sp *ServiceProvider) Register(app foundation.Application) {
	app.Singleton("ratelimit", func(app foundation.Application) (any, error) {
		store := app.MustMake("config").(contracts.Config).GetString("ratelimit.store", "")
		return New(app.MustMake("cache").(contracts.Cache), store), nil
	})
}

func (sp *ServiceProvider) Boot(app foundation.Application) error {
	return nil
}

// ConfigDefaults 声明 ratelimit 的默认配置。
func (sp *ServiceProvider) ConfigDefaults() map[string]any {
	return map[string]any{
		"ratelimit.store": "",
	}
}
