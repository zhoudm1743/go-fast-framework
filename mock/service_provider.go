package mock

import "github.com/zhoudm1743/go-fast-framework/foundation"

// ServiceProvider Mock 服务提供者。
// 注意：该 Provider 通常在测试环境中注册，生产环境可选。
type ServiceProvider struct{}

func (sp *ServiceProvider) Register(app foundation.Application) {
	app.Singleton("mock", func(app foundation.Application) (any, error) {
		return NewManager(app), nil
	})
}

func (sp *ServiceProvider) Boot(app foundation.Application) error {
	return nil
}
