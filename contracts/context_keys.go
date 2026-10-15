package contracts

// CtxKey 框架 Context 中存储自定义值所用的键类型。
type CtxKey string

const (
	// CtxKeyUserID 当前请求的登录用户 ID。
	// gormdriver audit callback 读取此 key 自动填充 created_by / updated_by。
	CtxKeyUserID CtxKey = "user_id"

	// CtxKeyOrgNodeID 当前用户绑定的组织节点 ID（多级租户场景）。
	CtxKeyOrgNodeID CtxKey = "org_node_id"

	// CtxKeyTenantSchema 当前请求解析出的租户 schema 名。
	// 中间件将 schema 写入 Context 后，由 DB.Tenant(ctx) 读取并设置查询的 search_path。
	CtxKeyTenantSchema CtxKey = "tenant_schema"

	// CtxKeyAuthUser 当前请求的已认证用户对象（contracts.AuthUser）。
	// 认证中间件写入此 key 后，Guard.User(ctx) 直接命中，无需重复解析凭证。
	CtxKeyAuthUser CtxKey = "auth_user"

	// CtxKeyRadarSpan 当前请求的 radar Span（HTTP 埋点中间件写入）。
	CtxKeyRadarSpan CtxKey = "radar_span"

	// CtxKeyRadarStdCtx 携带当前 Span 的标准 context.Context（HTTP 埋点中间件写入）。
	// 业务侧用 radar.StdCtx(ctx) 取出后开子 Span。
	CtxKeyRadarStdCtx CtxKey = "radar_std_ctx"
)
