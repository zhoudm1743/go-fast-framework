package facades

import "github.com/zhoudm1743/go-fast-framework/contracts"

// Tenant 获取租户上下文解析器。
//
// 通常业务不需要直接调用此 Facade，而是用 facades.DB().Tenant(ctx) 直接拿到
// 已自动带 schema 的查询构建器。仅当需要裸 schema 字符串（如 Raw SQL 拼接）
// 时使用 facades.Tenant().SchemaFromCtx(ctx)。
func Tenant() contracts.TenantResolver {
	return App().MustMake("tenant").(contracts.TenantResolver)
}
