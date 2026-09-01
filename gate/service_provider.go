package gate

import "github.com/zhoudm1743/go-fast-framework/foundation"

// ServiceProvider Gate 服务提供者。
type ServiceProvider struct{}

func (sp *ServiceProvider) Register(app foundation.Application) {
	app.Singleton("gate", func(app foundation.Application) (any, error) {
		return NewGate(), nil
	})
}

func (sp *ServiceProvider) Boot(app foundation.Application) error {
	return nil
}
