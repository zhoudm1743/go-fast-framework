package contracts

import "time"

// ── 限流器契约 ──────────────────────────────────────────────────────

// RateLimiter 限流器契约（facades.RateLimit() 返回此接口）。
// 固定窗口算法，计数存于 cache（可通过 ratelimit.store 指定 cache store，默认默认 store）。
type RateLimiter interface {
	// Allow 在 window 窗口内最多放行 limit 次；返回是否放行与剩余次数。
	Allow(key string, limit int, window time.Duration) (allowed bool, remaining int64)
	// Hit 记录一次命中并返回窗口内当前计数（不判断是否超限）。
	Hit(key string, window time.Duration) (int64, error)
	// TooManyAttempts 当前是否已超限（不增加计数）。
	TooManyAttempts(key string, limit int, window time.Duration) bool
	// Attempts 当前窗口已命中次数。
	Attempts(key string) int64
	// AvailableIn 距离窗口重置的剩余时间；窗口未开始（无计数）返回 0。
	AvailableIn(key string) time.Duration
	// Clear 清除指定 key 的计数与计时。
	Clear(key string) error
}
