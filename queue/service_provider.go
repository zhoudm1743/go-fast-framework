package queue

import (
	"github.com/zhoudm1743/go-fast-framework/foundation"
)

// ServiceProvider 队列服务提供者。
type ServiceProvider struct{}

func (sp *ServiceProvider) Register(app foundation.Application) {
	app.Singleton("queue", func(app foundation.Application) (any, error) {
		return New(app.Log()), nil
	})
}

func (sp *ServiceProvider) Boot(app foundation.Application) error {
	// 优雅关闭：与 cache 一致，采用惰性模式，仅当队列已构造时才 Stop。
	app.OnShutdown(func() {
		if q, err := app.Make("queue"); err == nil {
			if m, ok := q.(*manager); ok {
				m.Stop()
			}
		}
	})
	return nil
}
