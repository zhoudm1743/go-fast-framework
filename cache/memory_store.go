package cache

import (
	"fmt"
	"hash/fnv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

const defaultShardCount = 32

type entry struct {
	value    any
	expireAt int64
}

func (e *entry) expired() bool {
	return e.expireAt > 0 && time.Now().UnixNano() > e.expireAt
}

type shard struct {
	mu    sync.RWMutex
	items map[string]*entry
	hash  map[string]map[string]any
}

func newShard() *shard {
	return &shard{
		items: make(map[string]*entry),
		hash:  make(map[string]map[string]any),
	}
}

type memoryStore struct {
	shards     []*shard
	shardCount uint32
	tags       map[string]map[string]struct{}
	tagMu      sync.RWMutex
	locks      sync.Map
	stopGC     chan struct{}
}

func NewMemoryStore(shardCount int, gcInterval time.Duration) *memoryStore {
	if shardCount <= 0 {
		shardCount = defaultShardCount
	}
	s := &memoryStore{
		shards:     make([]*shard, shardCount),
		shardCount: uint32(shardCount),
		tags:       make(map[string]map[string]struct{}),
		stopGC:     make(chan struct{}),
	}
	for i := range s.shards {
		s.shards[i] = newShard()
	}
	if gcInterval > 0 {
		go s.gc(gcInterval)
	}
	return s
}

func (s *memoryStore) getShard(key string) *shard {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return s.shards[h.Sum32()%s.shardCount]
}

func (s *memoryStore) gc(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			for _, sh := range s.shards {
				sh.mu.Lock()
				for k, e := range sh.items {
					if e.expired() {
						delete(sh.items, k)
					}
				}
				sh.mu.Unlock()
			}
		case <-s.stopGC:
			return
		}
	}
}

func (s *memoryStore) Stop() {
	select {
	case <-s.stopGC:
	default:
		close(s.stopGC)
	}
}

func (s *memoryStore) Get(key string, def ...any) any {
	sh := s.getShard(key)
	sh.mu.RLock()
	e, ok := sh.items[key]
	sh.mu.RUnlock()
	if !ok || e.expired() {
		if len(def) > 0 {
			return def[0]
		}
		return nil
	}
	return e.value
}

func (s *memoryStore) GetBool(key string, def ...bool) bool {
	v := s.Get(key)
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

func (s *memoryStore) GetInt(key string, def ...int) int {
	v := s.Get(key)
	if v == nil {
		if len(def) > 0 {
			return def[0]
		}
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func (s *memoryStore) GetInt64(key string, def ...int64) int64 {
	v := s.Get(key)
	if v == nil {
		if len(def) > 0 {
			return def[0]
		}
		return 0
	}
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	default:
		return 0
	}
}

func (s *memoryStore) GetFloat64(key string, def ...float64) float64 {
	v := s.Get(key)
	if v == nil {
		if len(def) > 0 {
			return def[0]
		}
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	default:
		return 0
	}
}

func (s *memoryStore) GetString(key string, def ...string) string {
	v := s.Get(key)
	if v == nil {
		if len(def) > 0 {
			return def[0]
		}
		return ""
	}
	if str, ok := v.(string); ok {
		return str
	}
	return fmt.Sprintf("%v", v)
}

func (s *memoryStore) Has(key string) bool {
	sh := s.getShard(key)
	sh.mu.RLock()
	e, ok := sh.items[key]
	sh.mu.RUnlock()
	return ok && !e.expired()
}

func (s *memoryStore) Put(key string, value any, ttl time.Duration) error {
	sh := s.getShard(key)
	var expireAt int64
	if ttl > 0 {
		expireAt = time.Now().Add(ttl).UnixNano()
	}
	sh.mu.Lock()
	sh.items[key] = &entry{value: value, expireAt: expireAt}
	sh.mu.Unlock()
	return nil
}

func (s *memoryStore) Forever(key string, value any) error {
	return s.Put(key, value, 0)
}

func (s *memoryStore) Forget(key string) error {
	sh := s.getShard(key)
	sh.mu.Lock()
	delete(sh.items, key)
	sh.mu.Unlock()
	s.tagMu.Lock()
	for _, keys := range s.tags {
		delete(keys, key)
	}
	s.tagMu.Unlock()
	return nil
}

func (s *memoryStore) Flush() error {
	for _, sh := range s.shards {
		sh.mu.Lock()
		sh.items = make(map[string]*entry)
		sh.hash = make(map[string]map[string]any)
		sh.mu.Unlock()
	}
	s.tagMu.Lock()
	s.tags = make(map[string]map[string]struct{})
	s.tagMu.Unlock()
	return nil
}

func (s *memoryStore) Pull(key string, def ...any) any {
	v := s.Get(key, def...)
	_ = s.Forget(key)
	return v
}

func (s *memoryStore) Increment(key string, value ...int64) (int64, error) {
	delta := int64(1)
	if len(value) > 0 {
		delta = value[0]
	}
	sh := s.getShard(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.items[key]
	if !ok || e.expired() {
		sh.items[key] = &entry{value: delta, expireAt: 0}
		return delta, nil
	}
	cur, ok := toInt64(e.value)
	if !ok {
		return 0, fmt.Errorf("[GoFast] cache: increment non-numeric value for key %q", key)
	}
	n := cur + delta
	e.value = n
	return n, nil
}

func (s *memoryStore) Decrement(key string, value ...int64) (int64, error) {
	delta := int64(1)
	if len(value) > 0 {
		delta = value[0]
	}
	return s.Increment(key, -delta)
}

func (s *memoryStore) Remember(key string, ttl time.Duration, callback func() (any, error)) (any, error) {
	if v := s.Get(key); v != nil {
		return v, nil
	}
	v, err := callback()
	if err != nil {
		return nil, err
	}
	_ = s.Put(key, v, ttl)
	return v, nil
}

func (s *memoryStore) RememberForever(key string, callback func() (any, error)) (any, error) {
	return s.Remember(key, 0, callback)
}

func (s *memoryStore) Many(keys []string) map[string]any {
	result := make(map[string]any, len(keys))
	for _, k := range keys {
		result[k] = s.Get(k)
	}
	return result
}

func (s *memoryStore) PutMany(values map[string]any, ttl time.Duration) error {
	for k, v := range values {
		if err := s.Put(k, v, ttl); err != nil {
			return err
		}
	}
	return nil
}

func (s *memoryStore) Tags(tags ...string) contracts.TaggedCache {
	return &taggedCache{store: s, tags: tags}
}

func (s *memoryStore) HSet(key, field string, value any) error {
	sh := s.getShard(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	if sh.hash[key] == nil {
		sh.hash[key] = make(map[string]any)
	}
	sh.hash[key][field] = value
	return nil
}

func (s *memoryStore) HGet(key, field string) (any, error) {
	sh := s.getShard(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	if m, ok := sh.hash[key]; ok {
		if v, ok := m[field]; ok {
			return v, nil
		}
	}
	return nil, nil
}

func (s *memoryStore) HDel(key string, fields ...string) error {
	sh := s.getShard(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	if m, ok := sh.hash[key]; ok {
		for _, f := range fields {
			delete(m, f)
		}
		if len(m) == 0 {
			delete(sh.hash, key)
		}
	}
	return nil
}

func (s *memoryStore) HExists(key, field string) bool {
	sh := s.getShard(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	if m, ok := sh.hash[key]; ok {
		_, exists := m[field]
		return exists
	}
	return false
}

func (s *memoryStore) HGetAll(key string) (map[string]any, error) {
	sh := s.getShard(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	m, ok := sh.hash[key]
	if !ok {
		return nil, nil
	}
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result, nil
}

func (s *memoryStore) HLen(key string) int64 {
	sh := s.getShard(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	return int64(len(sh.hash[key]))
}

func (s *memoryStore) HKeys(key string) ([]string, error) {
	sh := s.getShard(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	m, ok := sh.hash[key]
	if !ok {
		return nil, nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys, nil
}

func (s *memoryStore) Lock(key string, ttl time.Duration) contracts.CacheLock {
	actual, _ := s.locks.LoadOrStore(key, &memoryLock{key: key, ttl: ttl})
	return actual.(*memoryLock)
}

func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case int32:
		return int64(n), true
	case float64:
		return int64(n), true
	default:
		return 0, false
	}
}

type memoryLock struct {
	key      string
	ttl      time.Duration
	acquired int32
}

func (l *memoryLock) Acquire() bool {
	return atomic.CompareAndSwapInt32(&l.acquired, 0, 1)
}

func (l *memoryLock) Release() bool {
	return atomic.CompareAndSwapInt32(&l.acquired, 1, 0)
}

func (l *memoryLock) ForceRelease() bool {
	atomic.StoreInt32(&l.acquired, 0)
	return true
}

func (l *memoryLock) Block(timeout time.Duration, callback ...func()) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if l.Acquire() {
			for _, cb := range callback {
				cb()
			}
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

type taggedCache struct {
	store *memoryStore
	tags  []string
}

func (t *taggedCache) track(key string) {
	t.store.tagMu.Lock()
	defer t.store.tagMu.Unlock()
	for _, tag := range t.tags {
		if t.store.tags[tag] == nil {
			t.store.tags[tag] = make(map[string]struct{})
		}
		t.store.tags[tag][key] = struct{}{}
	}
}

func (t *taggedCache) Get(key string, def ...any) any    { return t.store.Get(key, def...) }
func (t *taggedCache) Has(key string) bool               { return t.store.Has(key) }
func (t *taggedCache) Many(keys []string) map[string]any { return t.store.Many(keys) }

func (t *taggedCache) Put(key string, value any, ttl time.Duration) error {
	t.track(key)
	return t.store.Put(key, value, ttl)
}

func (t *taggedCache) Forever(key string, value any) error {
	t.track(key)
	return t.store.Forever(key, value)
}

func (t *taggedCache) Forget(key string) error { return t.store.Forget(key) }

func (t *taggedCache) PutMany(values map[string]any, ttl time.Duration) error {
	for k := range values {
		t.track(k)
	}
	return t.store.PutMany(values, ttl)
}

func (t *taggedCache) Increment(key string, value ...int64) (int64, error) {
	t.track(key)
	return t.store.Increment(key, value...)
}

func (t *taggedCache) Decrement(key string, value ...int64) (int64, error) {
	t.track(key)
	return t.store.Decrement(key, value...)
}

func (t *taggedCache) Flush() error {
	t.store.tagMu.Lock()
	keysToDelete := make(map[string]struct{})
	for _, tag := range t.tags {
		for k := range t.store.tags[tag] {
			keysToDelete[k] = struct{}{}
		}
		delete(t.store.tags, tag)
	}
	t.store.tagMu.Unlock()
	for k := range keysToDelete {
		_ = t.store.Forget(k)
	}
	return nil
}
