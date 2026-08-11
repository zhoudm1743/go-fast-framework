package facades

import (
	"github.com/zhoudm1743/go-fast-framework/contracts"
	gosession "github.com/zhoudm1743/go-fast-framework/http/session"
)

// Http 是 HTTP 层相关服务的门面命名空间，聚合了 Route、Session、JWT、Validator 四个能力。
//
// 使用示例：
//
//	facades.Http.Route().GET("/ping", handler)
//	facades.Http.JWT().Sign(claims, key)
//	facades.Http.Validator().Validate(input)
//	facades.Http.Session().Middleware()
//	sess, err := facades.Http.SessionFor(ctx)
var Http = &httpFacade{}

type httpFacade struct{}

// Session 返回 Session 管理器，可用于注册中间件或手动操作会话。
func (h *httpFacade) Session() *gosession.Manager {
	return App().MustMake("session").(*gosession.Manager)
}

// SessionFor 从当前请求上下文中获取已加载的 Session 对象。
// 通常在通过 Http.Session().Middleware() 注册中间件后，在控制器中直接调用。
func (h *httpFacade) SessionFor(ctx contracts.Context) (contracts.Session, error) {
	return h.Session().Session(ctx)
}

// JWT 返回 JWT 服务实例，用于签发和解析 Token。
func (h *httpFacade) JWT() contracts.JWT {
	return App().MustMake("jwt").(contracts.JWT)
}

// Validator 返回请求验证服务实例。
func (h *httpFacade) Validator() contracts.Validation {
	return App().MustMake("validator").(contracts.Validation)
}

// Route 返回路由服务实例，用于注册 HTTP 路由和中间件。
func (h *httpFacade) Route() contracts.Route {
	return App().MustMake("route").(contracts.Route)
}
