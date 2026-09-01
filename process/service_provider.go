package process

import "github.com/zhoudm1743/go-fast-framework/foundation"

// ServiceProvider Process 服务提供者。
type ServiceProvider struct{}

func (sp *ServiceProvider) Register(app foundation.Application) {
	app.Singleton("process", func(app foundation.Application) (any, error) {
		return NewProcess(), nil
	})
}

func (sp *ServiceProvider) Boot(app foundation.Application) error {
	return nil
}
