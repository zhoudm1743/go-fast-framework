package middleware

import (
	"math"
	"strconv"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/facades"
)

// RateLimit 限流中间件：window 内最多 limit 次。
// keyFn 缺省为 "IP + FullPath"；超限返回 429 并带 Retry-After 头。
func RateLimit(limit int, window time.Duration, keyFn ...func(contracts.Context) string) contracts.HandlerFunc {
	return func(ctx contracts.Context) error {
		key := ctx.IP() + ctx.FullPath()
		if len(keyFn) > 0 && keyFn[0] != nil {
			key = keyFn[0](ctx)
		}

		limiter := facades.RateLimit()
		if allowed, _ := limiter.Allow(key, limit, window); !allowed {
			ctx.SetHeader("Retry-After", retryAfter(limiter.AvailableIn(key)))
			return ctx.Response().Fail(429, "请求过于频繁")
		}
		return ctx.Next()
	}
}

// retryAfter 将剩余窗口时间换算为 Retry-After 秒数（向上取整，至少 1）。
func retryAfter(d time.Duration) string {
	secs := int64(math.Ceil(d.Seconds()))
	if secs < 1 {
		secs = 1
	}
	return strconv.FormatInt(secs, 10)
}
