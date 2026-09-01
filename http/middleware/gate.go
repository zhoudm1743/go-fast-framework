package middleware

import (
	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/facades"
)

// Can 创建一个检查指定 Gate 能力的中间件。
// 若检查失败，返回 403 Forbidden。
func Can(ability string, args ...any) contracts.HandlerFunc {
	return func(ctx contracts.Context) error {
		if facades.Gate().Allows(ctx, ability, args...) {
			return ctx.Next()
		}
		return ctx.Response().Forbidden()
	}
}

// Cannot 创建一个反向检查中间件：具备指定能力时返回 403。
func Cannot(ability string, args ...any) contracts.HandlerFunc {
	return func(ctx contracts.Context) error {
		if facades.Gate().Denies(ctx, ability, args...) {
			return ctx.Next()
		}
		return ctx.Response().Forbidden()
	}
}
