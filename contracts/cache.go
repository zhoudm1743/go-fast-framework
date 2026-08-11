package contracts

import "time"

// ── 缓存服务 ────────────────────────────────────────────────────────

// Cache 缓存服务契约。支持多 Store 切换、标签分组、原子操作、分布式锁。
type Cache interface {
	CacheStore

	// Store 获取指定驱动的缓存实例（如 "memory"、"redis"、"file"）。
	Store(name string) CacheStore
}

// CacheStore 缓存存储契约（单个驱动实例的完整能力）。
type CacheStore interface {
	// ── 基础 CRUD ──────────────────────────────────────────────

	// Get 获取缓存值，不存在返回 def（可选）。
	Get(key string, def ...any) any
	// GetBool / GetInt / GetInt64 / GetFloat64 / GetString 类型化获取。
	GetBool(key string, def ...bool) bool
	GetInt(key string, def ...int) int
	GetInt64(key string, def ...int64) int64
	GetFloat64(key string, def ...float64) float64
	GetString(key string, def ...string) string
	// Has 判断 key 是否存在。
	Has(key string) bool
	// Put 设置缓存值和 TTL。TTL <= 0 表示永不过期。
	Put(key string, value any, ttl time.Duration) error
	// Forever 设置永不过期的缓存。
	Forever(key string, value any) error
	// Forget 删除缓存。
	Forget(key string) error
	// Flush 清空全部缓存。
	Flush() error

	// Pull 获取后删除。
	Pull(key string, def ...any) any

	// ── 原子操作 ───────────────────────────────────────────────

	// Increment 原子自增，返回自增后的值。
	Increment(key string, value ...int64) (int64, error)
	// Decrement 原子自减，返回自减后的值。
	Decrement(key string, value ...int64) (int64, error)

	// Remember 存在则返回缓存，不存在则调用 callback 生成值并缓存。
	Remember(key string, ttl time.Duration, callback func() (any, error)) (any, error)
	// RememberForever 同 Remember，但永不过期。
	RememberForever(key string, callback func() (any, error)) (any, error)

	// ── 批量操作 ───────────────────────────────────────────────

	// Many 批量获取。
	Many(keys []string) map[string]any
	// PutMany 批量设置。
	PutMany(values map[string]any, ttl time.Duration) error

	// ── 标签分组 ───────────────────────────────────────────────

	// Tags 按标签分组。对返回的 TaggedCache 操作时，所有 key 会关联这些 tag。
	// 可通过 tag 批量清除同组缓存。
	Tags(tags ...string) TaggedCache

	// ── Hash 操作 ──────────────────────────────────────────────

	// HGet 获取 hash 字段值。
	HGet(key, field string) (any, error)
	// HSet 设置 hash 字段值。
	HSet(key, field string, value any) error
	// HDel 删除 hash 字段。
	HDel(key string, fields ...string) error
	// HExists 判断 hash 字段是否存在。
	HExists(key, field string) bool
	// HGetAll 获取 hash 所有字段。
	HGetAll(key string) (map[string]any, error)
	// HLen 获取 hash 字段数量。
	HLen(key string) int64
	// HKeys 获取 hash 所有字段名。
	HKeys(key string) ([]string, error)

	// ── 分布式锁 ──────────────────────────────────────────────

	// Lock 获取分布式锁。
	Lock(key string, ttl time.Duration) CacheLock
}

// TaggedCache 带标签的缓存操作。
type TaggedCache interface {
	// 继承基础读写
	Get(key string, def ...any) any
	Has(key string) bool
	Put(key string, value any, ttl time.Duration) error
	Forever(key string, value any) error
	Forget(key string) error
	// Flush 清除该标签组下所有缓存。
	Flush() error
	// Many / PutMany 批量操作。
	Many(keys []string) map[string]any
	PutMany(values map[string]any, ttl time.Duration) error
	// Increment / Decrement 原子操作。
	Increment(key string, value ...int64) (int64, error)
	Decrement(key string, value ...int64) (int64, error)
}

// CacheLock 分布式锁契约。
type CacheLock interface {
	// Acquire 尝试获取锁，返回是否成功。
	Acquire() bool
	// Release 释放锁。
	Release() bool
	// ForceRelease 强制释放锁。
	ForceRelease() bool
	// Block 在 timeout 内阻塞等待获取锁；获取后执行 callback。
	Block(timeout time.Duration, callback ...func()) bool
}
