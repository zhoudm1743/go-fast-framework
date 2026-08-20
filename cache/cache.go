package cache

import (
	"log"
	"time"

	fileStore "github.com/zhoudm1743/go-fast-framework/cache/file_store"
	redisStore "github.com/zhoudm1743/go-fast-framework/cache/redis_store"
	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// cacheManager 实现 contracts.Cache，管理多个 CacheStore 实例。
type cacheManager struct {
	stores       map[string]contracts.CacheStore
	defaultStore string
}

// NewCacheManager 根据配置创建缓存管理器。
//
// 驱动策略（降级链 redis → file → memory）：
//   - 未配置 cache.driver 时默认使用 file（跨重启持久化，无外部依赖）
//   - 配置了 cache.redis（host 或 driver=redis）则优先 redis；连接失败自动降级为 file
//   - file 目录不可用（权限/路径非法）时降级为 memory
//   - memory 为最终兜底，总是可用
//
// 降级只影响默认 store，用户仍可通过 Store("redis")/Store("file") 显式使用可用驱动。
func NewCacheManager(cfg contracts.Config) (contracts.Cache, error) {
	driver := cfg.GetString("cache.driver", "file")

	m := &cacheManager{
		stores:       make(map[string]contracts.CacheStore),
		defaultStore: "memory",
	}

	// memory store：最终兜底，总是可用
	shardCount := cfg.GetInt("cache.memory.shard_count", 32)
	gcSec := cfg.GetInt("cache.memory.clean_interval", 60)
	m.stores["memory"] = NewMemoryStore(shardCount, time.Duration(gcSec)*time.Second)

	// file store：默认驱动与 redis 的降级目标，总是尝试初始化
	filePath := cfg.GetString("cache.file.path", "storage/cache")
	fileGCSec := cfg.GetInt("cache.file.clean_interval", 600)
	fs, fileErr := fileStore.New(filePath, time.Duration(fileGCSec)*time.Second)
	if fileErr == nil {
		m.stores["file"] = fs
	}

	// redis store：仅在配置了 host 或期望驱动为 redis 时尝试连接
	redisAttempted := false
	var redisErr error
	if cfg.GetString("cache.redis.host", "") != "" || driver == "redis" {
		redisAttempted = true
		redisCfg := redisStore.Config{
			Host:     cfg.GetString("cache.redis.host", "127.0.0.1"),
			Port:     cfg.GetInt("cache.redis.port", 6379),
			Password: cfg.GetString("cache.redis.password", ""),
			DB:       cfg.GetInt("cache.redis.db", 0),
			Prefix:   cfg.GetString("cache.redis.prefix", ""),
		}
		rs, err := redisStore.New(redisCfg)
		if err != nil {
			redisErr = err
		} else {
			m.stores["redis"] = rs
		}
	}

	// 按降级链决定实际默认 store
	switch driver {
	case "redis":
		if _, ok := m.stores["redis"]; ok {
			m.defaultStore = "redis"
		} else if _, ok := m.stores["file"]; ok {
			log.Printf("[GoFast][cache] 警告: redis 缓存连接失败（%v），已降级为文件缓存", redisErr)
			m.defaultStore = "file"
		} else {
			log.Printf("[GoFast][cache] 警告: redis 缓存连接失败（%v），文件缓存不可用（%v），已降级为内存缓存", redisErr, fileErr)
		}
	case "file":
		if _, ok := m.stores["file"]; ok {
			m.defaultStore = "file"
		} else {
			log.Printf("[GoFast][cache] 警告: 文件缓存不可用（%v），已降级为内存缓存", fileErr)
		}
	default:
		// 显式配置 memory 或其他未知驱动：默认 store 保持 memory
	}

	// redis 已配置但连接失败，且期望驱动不是 redis（switch 已处理 driver=redis 的降级警告）时，提醒用户
	if redisAttempted && redisErr != nil && driver != "redis" {
		log.Printf("[GoFast][cache] 警告: redis 缓存连接失败，Store(\"redis\") 不可用: %v", redisErr)
	}

	return m, nil
}

// Stop 停止所有可停止的 store。
func (m *cacheManager) Stop() {
	for _, s := range m.stores {
		switch st := s.(type) {
		case *memoryStore:
			st.Stop()
		case *fileStore.FileStore:
			st.Stop()
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
