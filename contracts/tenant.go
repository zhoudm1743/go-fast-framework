package contracts

import "sync/atomic"

// TenantResolver 租户上下文解析器。
// 业务层只依赖此接口，不关心 SaaS / 独立版差异。
type TenantResolver interface {
	// SchemaFromCtx 返回当前请求应访问的 PostgreSQL schema 名。
	// standalone 模式恒返回 "public"。
	SchemaFromCtx(ctx Context) string
}

var globalTenantResolver atomic.Value

// RegisterTenantResolver 注册全局租户解析器。
// 由业务层 ServiceProvider 在 Register 阶段注入。
func RegisterTenantResolver(r TenantResolver) {
	if r == nil {
		return
	}
	globalTenantResolver.Store(r)
}

// GlobalTenantResolver 返回已注册的租户解析器，若未注册返回 nil。
func GlobalTenantResolver() TenantResolver {
	if v := globalTenantResolver.Load(); v != nil {
		if r, ok := v.(TenantResolver); ok {
			return r
		}
	}
	return nil
}
