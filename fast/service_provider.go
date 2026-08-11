package fast

import (
	"github.com/zhoudm1743/go-fast-framework/foundation"
)

const fastKey = "fast"

// ServiceProvider Fast 服务提供者，向容器注册 "fast" 服务。
type ServiceProvider struct{}

func (sp *ServiceProvider) Register(app foundation.Application) {
	app.Singleton(fastKey, func(_ foundation.Application) (any, error) {
		return newKernel(), nil
	})
}

func (sp *ServiceProvider) Boot(_ foundation.Application) error {
	return nil
}
