// Package tenant 提供租户上下文解析服务（SaaS / 独立版通过构建标签切换实现）。
package tenant

import (
	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/foundation"
)

type ServiceProvider struct{}

func (sp *ServiceProvider) Register(app foundation.Application) {
	r := newResolver()
	app.Instance("tenant", r)
	// 同时注册为全局解析器，供底层（database、utils 等）在不依赖容器的情况下访问。
	contracts.RegisterTenantResolver(r)
}

func (sp *ServiceProvider) Boot(foundation.Application) error {
	return nil
}
