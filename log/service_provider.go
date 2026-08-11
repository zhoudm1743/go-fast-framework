package log

import (
	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/foundation"
)

// ServiceProvider Log 服务提供者。
type ServiceProvider struct{}

func (sp *ServiceProvider) Register(app foundation.Application) {
	app.Singleton("log", func(app foundation.Application) (any, error) {
		cfg := app.MustMake("config").(contracts.Config)
		return NewLogger(cfg)
	})
}

func (sp *ServiceProvider) Boot(app foundation.Application) error {
	// 注册优雅关闭钩子：刷新日志缓冲区并关闭 lumberjack writer
	app.OnShutdown(func() {
		if closer, ok := app.Log().(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	})
	return nil
}
