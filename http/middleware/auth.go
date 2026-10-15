package middleware

import (
	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/facades"
)

// Auth 认证中间件：解析凭证、加载当前用户并写入 ctx，失败返回 401。
// 不传 guard 使用默认守卫；可指定命名守卫（如 middleware.Auth("admin")）。
//
// 认证成功后：
//   - ctx 写入 contracts.CtxKeyUserID（gate 授权与 ORM 审计自动生效）
//   - ctx 写入 contracts.CtxKeyAuthUser（Guard.User(ctx) 直接命中，免重复解析）
func Auth(guard ...string) contracts.HandlerFunc {
	return func(ctx contracts.Context) error {
		user, err := facades.Auth().Guard(guard...).User(ctx)
		if err != nil {
			return ctx.Response().Unauthorized("未认证")
		}
		ctx.WithValue(string(contracts.CtxKeyUserID), user.GetAuthID())
		ctx.WithValue(string(contracts.CtxKeyAuthUser), user)
		return ctx.Next()
	}
}
