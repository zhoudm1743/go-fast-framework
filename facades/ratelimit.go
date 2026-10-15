package facades

import "github.com/zhoudm1743/go-fast-framework/contracts"

// RateLimit 获取限流服务实例。
func RateLimit() contracts.RateLimiter {
	return App().MustMake("ratelimit").(contracts.RateLimiter)
}
