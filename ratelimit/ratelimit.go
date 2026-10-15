package ratelimit

import (
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// keyPrefix 限流键统一前缀。
const keyPrefix = "ratelimit:"

// rateLimiter 固定窗口限流器实现。
type rateLimiter struct {
	cache     contracts.Cache
	storeName string
}

// New 创建固定窗口限流器。
// storeName 为缓存 store 名（空字符串使用默认 store）。
func New(cache contracts.Cache, storeName string) contracts.RateLimiter {
	return &rateLimiter{cache: cache, storeName: storeName}
}

// store 返回实际写入计数的 cache store。
func (l *rateLimiter) store() contracts.CacheStore {
	if l.storeName != "" {
		return l.cache.Store(l.storeName)
	}
	return l.cache
}

// countKey 返回计数键：ratelimit:<key>。
func countKey(key string) string { return keyPrefix + key }

// timerKey 返回计时键：ratelimit:<key>:timer。
func timerKey(key string) string { return keyPrefix + key + ":timer" }

// Allow 在 window 窗口内最多放行 limit 次；返回是否放行与剩余次数。
func (l *rateLimiter) Allow(key string, limit int, window time.Duration) (bool, int64) {
	count, err := l.Hit(key, window)
	if err != nil {
		return false, 0
	}
	remaining := int64(limit) - count
	if remaining < 0 {
		remaining = 0
	}
	return count <= int64(limit), remaining
}

// Hit 记录一次命中并返回窗口内当前计数（不判断是否超限）。
func (l *rateLimiter) Hit(key string, window time.Duration) (int64, error) {
	store := l.store()
	ck := countKey(key)

	// 先判断存在性：不存在则 Put 以带 TTL（Increment 新建的键无 TTL，会永久残留），
	// 存在则走 Increment 原子自增，避免并发首次命中漏写 TTL。
	if !store.Has(ck) {
		_ = store.Put(ck, int64(1), window)
		_ = store.Put(timerKey(key), time.Now().Add(window).Unix(), window)
		return 1, nil
	}
	count, err := store.Increment(ck)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// TooManyAttempts 当前是否已超限（不增加计数）。
func (l *rateLimiter) TooManyAttempts(key string, limit int, window time.Duration) bool {
	return l.Attempts(key) >= int64(limit)
}

// Attempts 当前窗口已命中次数。
func (l *rateLimiter) Attempts(key string) int64 {
	return l.store().GetInt64(countKey(key))
}

// AvailableIn 距离窗口重置的剩余时间；窗口未开始（无计数）返回 0。
func (l *rateLimiter) AvailableIn(key string) time.Duration {
	deadline := l.store().GetInt64(timerKey(key))
	if deadline <= 0 {
		return 0
	}
	remaining := time.Until(time.Unix(deadline, 0))
	if remaining < 0 {
		return 0
	}
	return remaining
}

// Clear 清除指定 key 的计数与计时。
func (l *rateLimiter) Clear(key string) error {
	store := l.store()
	_ = store.Forget(countKey(key))
	return store.Forget(timerKey(key))
}
