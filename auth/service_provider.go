package auth

import (
	"github.com/zhoudm1743/go-fast-framework/foundation"
)

// ServiceProvider Auth 服务提供者。
type ServiceProvider struct{}

func (sp *ServiceProvider) Register(app foundation.Application) {
	app.Singleton("auth", func(app foundation.Application) (any, error) {
		return NewManager(app), nil
	})
}

func (sp *ServiceProvider) Boot(app foundation.Application) error {
	return nil
}

// ConfigDefaults 声明 auth 的默认配置。
func (sp *ServiceProvider) ConfigDefaults() map[string]any {
	return map[string]any{
		"auth.default_guard": "api",
	}
}
