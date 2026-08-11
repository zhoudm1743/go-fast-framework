package redisStore

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/zhoudm1743/go-fast-framework/contracts"
)

var ctx = context.Background()

// RedisStore 基于 Redis 的缓存存储实现。
type RedisStore struct {
	client *redis.Client
	prefix string
	locks  sync.Map
}

// Config Redis 连接配置。
type Config struct {
	Host     string
	Port     int
	Password string
	DB       int
	Prefix   string
}

// New 创建 RedisStore 实例。
func New(cfg Config) (*RedisStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("[GoFast] redis: connect failed: %w", err)
	}
	return &RedisStore{
		client: client,
		prefix: cfg.Prefix,
	}, nil
}

func (r *RedisStore) key(k string) string {
	if r.prefix == "" {
		return k
	}
	return r.prefix + ":" + k
}

// serialize 将值序列化为 JSON。
func serialize(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

// deserialize 将 JSON 反序列化为 any。
func deserialize(s string) any {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return s
	}
	return v
}

func (r *RedisStore) Get(key string, def ...any) any {
	val, err := r.client.Get(ctx, r.key(key)).Result()
	if err != nil {
		if len(def) > 0 {
			return def[0]
		}
		return nil
	}
	return deserialize(val)
}

func (r *RedisStore) GetBool(key string, def ...bool) bool {
	v := r.Get(key)
	if v == nil {
		if len(def) > 0 {
			return def[0]
		}
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

func (r *RedisStore) GetInt(key string, def ...int) int {
	v := r.Get(key)
	if v == nil {
		if len(def) > 0 {
			return def[0]
		}
		return 0
	}
	return toInt(v)
}

func (r *RedisStore) GetInt64(key string, def ...int64) int64 {
	v := r.Get(key)
	if v == nil {
		if len(def) > 0 {
			return def[0]
		}
		return 0
	}
	return toInt64(v)
}

func (r *RedisStore) GetFloat64(key string, def ...float64) float64 {
	v := r.Get(key)
	if v == nil {
		if len(def) > 0 {
			return def[0]
		}
		return 0
	}
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}

func (r *RedisStore) GetString(key string, def ...string) string {
	v := r.Get(key)
	if v == nil {
		if len(def) > 0 {
			return def[0]
		}
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func (r *RedisStore) Has(key string) bool {
	n, err := r.client.Exists(ctx, r.key(key)).Result()
	return err == nil && n > 0
}

func (r *RedisStore) Put(key string, value any, ttl time.Duration) error {
	s, err := serialize(value)
	if err != nil {
		return err
	}
	if ttl <= 0 {
		return r.client.Set(ctx, r.key(key), s, 0).Err()
	}
	return r.client.Set(ctx, r.key(key), s, ttl).Err()
}

func (r *RedisStore) Forever(key string, value any) error {
	return r.Put(key, value, 0)
}

func (r *RedisStore) Forget(key string) error {
	return r.client.Del(ctx, r.key(key)).Err()
}

func (r *RedisStore) Flush() error {
	if r.prefix == "" {
		return r.client.FlushDB(ctx).Err()
	}
	// 只删除带前缀的 key
	var cursor uint64
	for {
		keys, cur, err := r.client.Scan(ctx, cursor, r.prefix+":*", 100).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := r.client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		cursor = cur
		if cursor == 0 {
			break
		}
	}
	return nil
}

func (r *RedisStore) Pull(key string, def ...any) any {
	v := r.Get(key, def...)
	_ = r.Forget(key)
	return v
}

func (r *RedisStore) Increment(key string, value ...int64) (int64, error) {
	delta := int64(1)
	if len(value) > 0 {
		delta = value[0]
	}
	return r.client.IncrBy(ctx, r.key(key), delta).Result()
}

func (r *RedisStore) Decrement(key string, value ...int64) (int64, error) {
	delta := int64(1)
	if len(value) > 0 {
		delta = value[0]
	}
	return r.client.DecrBy(ctx, r.key(key), delta).Result()
}

func (r *RedisStore) Remember(key string, ttl time.Duration, callback func() (any, error)) (any, error) {
	if v := r.Get(key); v != nil {
		return v, nil
	}
	v, err := callback()
	if err != nil {
		return nil, err
	}
	if err := r.Put(key, v, ttl); err != nil {
		return nil, err
	}
	return v, nil
}

func (r *RedisStore) RememberForever(key string, callback func() (any, error)) (any, error) {
	return r.Remember(key, 0, callback)
}

func (r *RedisStore) Many(keys []string) map[string]any {
	rkeys := make([]string, len(keys))
	for i, k := range keys {
		rkeys[i] = r.key(k)
	}
	vals, err := r.client.MGet(ctx, rkeys...).Result()
	result := make(map[string]any, len(keys))
	for i, k := range keys {
		if err != nil || vals[i] == nil {
			result[k] = nil
		} else {
			result[k] = deserialize(vals[i].(string))
		}
	}
	return result
}

func (r *RedisStore) PutMany(values map[string]any, ttl time.Duration) error {
	pipe := r.client.Pipeline()
	for k, v := range values {
		s, err := serialize(v)
		if err != nil {
			return err
		}
		pipe.Set(ctx, r.key(k), s, ttl)
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (r *RedisStore) Tags(tags ...string) contracts.TaggedCache {
	return &redisTaggedCache{store: r, tags: tags}
}

// ── Hash 操作 ──────────────────────────────────────────────────────────

func (r *RedisStore) HSet(key, field string, value any) error {
	s, err := serialize(value)
	if err != nil {
		return err
	}
	return r.client.HSet(ctx, r.key(key), field, s).Err()
}

func (r *RedisStore) HGet(key, field string) (any, error) {
	val, err := r.client.HGet(ctx, r.key(key), field).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return deserialize(val), nil
}

func (r *RedisStore) HDel(key string, fields ...string) error {
	return r.client.HDel(ctx, r.key(key), fields...).Err()
}

func (r *RedisStore) HExists(key, field string) bool {
	ok, err := r.client.HExists(ctx, r.key(key), field).Result()
	return err == nil && ok
}

func (r *RedisStore) HGetAll(key string) (map[string]any, error) {
	m, err := r.client.HGetAll(ctx, r.key(key)).Result()
	if err != nil {
		return nil, err
	}
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = deserialize(v)
	}
	return result, nil
}

func (r *RedisStore) HLen(key string) int64 {
	n, _ := r.client.HLen(ctx, r.key(key)).Result()
	return n
}

func (r *RedisStore) HKeys(key string) ([]string, error) {
	return r.client.HKeys(ctx, r.key(key)).Result()
}

// ── 分布式锁 ──────────────────────────────────────────────────────────

func (r *RedisStore) Lock(key string, ttl time.Duration) contracts.CacheLock {
	actual, _ := r.locks.LoadOrStore(key, &redisLock{
		client: r.client,
		key:    r.key("lock:" + key),
		ttl:    ttl,
	})
	return actual.(*redisLock)
}

// ── 辅助 ──────────────────────────────────────────────────────────────

func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	}
	return 0
}

// ── Redis 分布式锁 ────────────────────────────────────────────────────

type redisLock struct {
	client *redis.Client
	key    string
	ttl    time.Duration
}

func (l *redisLock) Acquire() bool {
	ok, err := l.client.SetNX(ctx, l.key, 1, l.ttl).Result()
	return err == nil && ok
}

func (l *redisLock) Release() bool {
	return l.client.Del(ctx, l.key).Err() == nil
}

func (l *redisLock) ForceRelease() bool {
	return l.Release()
}

func (l *redisLock) Block(timeout time.Duration, callback ...func()) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if l.Acquire() {
			for _, cb := range callback {
				cb()
			}
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// ── Redis TaggedCache ─────────────────────────────────────────────────

type redisTaggedCache struct {
	store *RedisStore
	tags  []string
}

func (t *redisTaggedCache) tagKey(tag string) string {
	return t.store.key("tag:" + tag)
}

func (t *redisTaggedCache) track(key string) {
	for _, tag := range t.tags {
		t.store.client.SAdd(ctx, t.tagKey(tag), key)
	}
}

func (t *redisTaggedCache) Get(key string, def ...any) any    { return t.store.Get(key, def...) }
func (t *redisTaggedCache) Has(key string) bool               { return t.store.Has(key) }
func (t *redisTaggedCache) Many(keys []string) map[string]any { return t.store.Many(keys) }

func (t *redisTaggedCache) Put(key string, value any, ttl time.Duration) error {
	t.track(key)
	return t.store.Put(key, value, ttl)
}

func (t *redisTaggedCache) Forever(key string, value any) error {
	t.track(key)
	return t.store.Forever(key, value)
}

func (t *redisTaggedCache) Forget(key string) error { return t.store.Forget(key) }

func (t *redisTaggedCache) PutMany(values map[string]any, ttl time.Duration) error {
	for k := range values {
		t.track(k)
	}
	return t.store.PutMany(values, ttl)
}

func (t *redisTaggedCache) Increment(key string, value ...int64) (int64, error) {
	t.track(key)
	return t.store.Increment(key, value...)
}

func (t *redisTaggedCache) Decrement(key string, value ...int64) (int64, error) {
	t.track(key)
	return t.store.Decrement(key, value...)
}

func (t *redisTaggedCache) Flush() error {
	for _, tag := range t.tags {
		tagKey := t.tagKey(tag)
		keys, err := t.store.client.SMembers(ctx, tagKey).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := t.store.client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		t.store.client.Del(ctx, tagKey)
	}
	return nil
}
