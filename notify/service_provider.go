package notify

import (
	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/foundation"
	"github.com/zhoudm1743/go-fast-framework/queue/redis_driver"
)

// ServiceProvider Notify 服务提供者。
type ServiceProvider struct{}

func (sp *ServiceProvider) Register(app foundation.Application) {
	// redis driver 按签名重建队列任务，需预注册 notifyJob 工厂。
	redis_driver.Register(notifyJobSignature, func() contracts.QueueJob {
		return notifyJob{}
	})

	app.Singleton("notify", func(app foundation.Application) (any, error) {
		return New(app), nil
	})
}

func (sp *ServiceProvider) Boot(app foundation.Application) error {
	return nil
}

// ConfigDefaults 声明 notify 的默认配置。
func (sp *ServiceProvider) ConfigDefaults() map[string]any {
	return map[string]any{
		cfgWebhookTimeout:       "10s",
		cfgWebhookRetry:         3,
		cfgWebhookRetryInterval: "1s",
	}
}
