package cache

import (
	"time"

	redisStore "github.com/zhoudm1743/go-fast-framework/cache/redis_store"
	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// cacheManager 实现 contracts.Cache，管理多个 CacheStore 实例。
type cacheManager struct {
	stores       map[string]contracts.CacheStore
	defaultStore string
}

// NewCacheManager 根据配置创建缓存管理器。
func NewCacheManager(cfg contracts.Config) (contracts.Cache, error) {
	defaultStore := cfg.GetString("cache.driver", "memory")
	shardCount := cfg.GetInt("cache.memory.shard_count", 32)
	gcSec := cfg.GetInt("cache.memory.clean_interval", 60)

	m := &cacheManager{
		stores:       make(map[string]contracts.CacheStore),
		defaultStore: defaultStore,
	}

	m.stores["memory"] = NewMemoryStore(shardCount, time.Duration(gcSec)*time.Second)

	// 如果配置了 Redis，初始化 Redis store
	if cfg.GetString("cache.redis.host", "") != "" || defaultStore == "redis" {
		redisCfg := redisStore.Config{
			Host:     cfg.GetString("cache.redis.host", "127.0.0.1"),
			Port:     cfg.GetInt("cache.redis.port", 6379),
			Password: cfg.GetString("cache.redis.password", ""),
			DB:       cfg.GetInt("cache.redis.db", 0),
			Prefix:   cfg.GetString("cache.redis.prefix", ""),
		}
		rs, err := redisStore.New(redisCfg)
		if err == nil {
			m.stores["redis"] = rs
		}
	}

	return m, nil
}

// Stop 停止所有可停止的 store。
func (m *cacheManager) Stop() {
	for _, s := range m.stores {
		if ms, ok := s.(*memoryStore); ok {
			ms.Stop()
		}
	}
}

func (m *cacheManager) Store(name string) contracts.CacheStore {
	if s, ok := m.stores[name]; ok {
		return s
	}
	return m.stores["memory"]
}

func (m *cacheManager) defaultCacheStore() contracts.CacheStore {
	return m.Store(m.defaultStore)
}

func (m *cacheManager) Get(key string, def ...any) any { return m.defaultCacheStore().Get(key, def...) }
func (m *cacheManager) GetBool(key string, def ...bool) bool {
	return m.defaultCacheStore().GetBool(key, def...)
}
func (m *cacheManager) GetInt(key string, def ...int) int {
	return m.defaultCacheStore().GetInt(key, def...)
}
func (m *cacheManager) GetInt64(key string, def ...int64) int64 {
	return m.defaultCacheStore().GetInt64(key, def...)
}
func (m *cacheManager) GetFloat64(key string, def ...float64) float64 {
	return m.defaultCacheStore().GetFloat64(key, def...)
}
func (m *cacheManager) GetString(key string, def ...string) string {
	return m.defaultCacheStore().GetString(key, def...)
}
func (m *cacheManager) Has(key string) bool { return m.defaultCacheStore().Has(key) }
func (m *cacheManager) Put(key string, value any, ttl time.Duration) error {
	return m.defaultCacheStore().Put(key, value, ttl)
}
func (m *cacheManager) Forever(key string, value any) error {
	return m.defaultCacheStore().Forever(key, value)
}
func (m *cacheManager) Forget(key string) error { return m.defaultCacheStore().Forget(key) }
func (m *cacheManager) Flush() error            { return m.defaultCacheStore().Flush() }
func (m *cacheManager) Pull(key string, def ...any) any {
	return m.defaultCacheStore().Pull(key, def...)
}
func (m *cacheManager) Increment(key string, v ...int64) (int64, error) {
	return m.defaultCacheStore().Increment(key, v...)
}
func (m *cacheManager) Decrement(key string, v ...int64) (int64, error) {
	return m.defaultCacheStore().Decrement(key, v...)
}
func (m *cacheManager) Remember(key string, ttl time.Duration, cb func() (any, error)) (any, error) {
	return m.defaultCacheStore().Remember(key, ttl, cb)
}
func (m *cacheManager) RememberForever(key string, cb func() (any, error)) (any, error) {
	return m.defaultCacheStore().RememberForever(key, cb)
}
func (m *cacheManager) Many(keys []string) map[string]any { return m.defaultCacheStore().Many(keys) }
func (m *cacheManager) PutMany(values map[string]any, ttl time.Duration) error {
	return m.defaultCacheStore().PutMany(values, ttl)
}
func (m *cacheManager) Tags(tags ...string) contracts.TaggedCache {
	return m.defaultCacheStore().Tags(tags...)
}
func (m *cacheManager) HGet(key, field string) (any, error) {
	return m.defaultCacheStore().HGet(key, field)
}
func (m *cacheManager) HSet(key, field string, value any) error {
	return m.defaultCacheStore().HSet(key, field, value)
}
func (m *cacheManager) HDel(key string, fields ...string) error {
	return m.defaultCacheStore().HDel(key, fields...)
}
func (m *cacheManager) HExists(key, field string) bool {
	return m.defaultCacheStore().HExists(key, field)
}
func (m *cacheManager) HGetAll(key string) (map[string]any, error) {
	return m.defaultCacheStore().HGetAll(key)
}
func (m *cacheManager) HLen(key string) int64              { return m.defaultCacheStore().HLen(key) }
func (m *cacheManager) HKeys(key string) ([]string, error) { return m.defaultCacheStore().HKeys(key) }
func (m *cacheManager) Lock(key string, ttl time.Duration) contracts.CacheLock {
	return m.defaultCacheStore().Lock(key, ttl)
}
