package event

import (
	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/foundation"
)

// ServiceProvider 事件服务提供者。
type ServiceProvider struct{}

func (sp *ServiceProvider) Register(app foundation.Application) {
	app.Singleton("event", func(app foundation.Application) (any, error) {
		return New(), nil
	})
}

func (sp *ServiceProvider) Boot(app foundation.Application) error {
	return nil
}

// ConfigDefaults 默认配置（实现 ConfigProvider）。
func (sp *ServiceProvider) ConfigDefaults() map[string]any {
	return map[string]any{}
}

// ── 辅助：供 bootstrap 快速注册事件 ──────────────────────────────────

// RegisterEvents 在引导完成后，通过 app 注册事件与监听器映射。
// 通常在 bootstrap/app.go 的 Boot() 函数中调用。
func RegisterEvents(app foundation.Application, events map[contracts.Eventer][]contracts.EventListener) {
	eventSvc := app.MustMake("event").(contracts.Event)
	eventSvc.Register(events)
}
