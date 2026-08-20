package cache

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/zhoudm1743/go-fast-framework/config"
)

func newTestStore() *memoryStore {
	return NewMemoryStore(16, 0) // 16 shards, no GC for tests
}

func TestPutGet(t *testing.T) {
	s := newTestStore()
	_ = s.Put("name", "GoFast", 5*time.Second)
	if s.GetString("name") != "GoFast" {
		t.Fatal("expected GoFast")
	}
}

func TestExpiry(t *testing.T) {
	s := newTestStore()
	_ = s.Put("k", "v", 50*time.Millisecond)
	time.Sleep(80 * time.Millisecond)
	if s.Has("k") {
		t.Fatal("key should have expired")
	}
}

func TestForever(t *testing.T) {
	s := newTestStore()
	_ = s.Forever("k", 42)
	if s.GetInt("k") != 42 {
		t.Fatal("expected 42")
	}
}

func TestPull(t *testing.T) {
	s := newTestStore()
	_ = s.Put("k", "v", 0)
	v := s.Pull("k")
	if v != "v" {
		t.Fatal("expected v")
	}
	if s.Has("k") {
		t.Fatal("key should be deleted after Pull")
	}
}

func TestIncrement(t *testing.T) {
	s := newTestStore()
	n, _ := s.Increment("counter")
	if n != 1 {
		t.Fatalf("expected 1, got %d", n)
	}
	n, _ = s.Increment("counter", 5)
	if n != 6 {
		t.Fatalf("expected 6, got %d", n)
	}
	n, _ = s.Decrement("counter", 2)
	if n != 4 {
		t.Fatalf("expected 4, got %d", n)
	}
}

func TestRemember(t *testing.T) {
	s := newTestStore()
	callCount := 0
	cb := func() (any, error) { callCount++; return "computed", nil }

	v1, _ := s.Remember("r", time.Minute, cb)
	v2, _ := s.Remember("r", time.Minute, cb)
	if v1 != "computed" || v2 != "computed" {
		t.Fatal("unexpected value")
	}
	if callCount != 1 {
		t.Fatalf("callback should be called once, got %d", callCount)
	}
}

func TestMany(t *testing.T) {
	s := newTestStore()
	_ = s.PutMany(map[string]any{"a": 1, "b": 2, "c": 3}, 0)
	result := s.Many([]string{"a", "b", "c", "d"})
	if result["a"] != 1 || result["b"] != 2 || result["c"] != 3 || result["d"] != nil {
		t.Fatalf("unexpected: %v", result)
	}
}

func TestFlush(t *testing.T) {
	s := newTestStore()
	_ = s.Put("k", "v", 0)
	_ = s.Flush()
	if s.Has("k") {
		t.Fatal("Flush should clear all")
	}
}

func TestHash(t *testing.T) {
	s := newTestStore()
	_ = s.HSet("user:1", "name", "Alice")
	_ = s.HSet("user:1", "age", 30)

	v, _ := s.HGet("user:1", "name")
	if v != "Alice" {
		t.Fatalf("expected Alice, got %v", v)
	}

	if !s.HExists("user:1", "age") {
		t.Fatal("age field should exist")
	}

	if s.HLen("user:1") != 2 {
		t.Fatalf("expected 2 fields, got %d", s.HLen("user:1"))
	}

	keys, _ := s.HKeys("user:1")
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}

	all, _ := s.HGetAll("user:1")
	if len(all) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(all))
	}

	_ = s.HDel("user:1", "age")
	if s.HExists("user:1", "age") {
		t.Fatal("age should be deleted")
	}
}

func TestTags(t *testing.T) {
	s := newTestStore()
	tagged := s.Tags("users", "api")
	_ = tagged.Put("u:1", "Alice", 0)
	_ = tagged.Put("u:2", "Bob", 0)
	_ = s.Put("other", "keep", 0)

	if tagged.Get("u:1") != "Alice" {
		t.Fatal("expected Alice")
	}

	// Flush only tagged keys
	_ = tagged.Flush()
	if s.Has("u:1") || s.Has("u:2") {
		t.Fatal("tagged keys should be flushed")
	}
	if !s.Has("other") {
		t.Fatal("untagged key should survive")
	}
}

func TestLock(t *testing.T) {
	s := newTestStore()
	lock := s.Lock("resource", time.Second)

	if !lock.Acquire() {
		t.Fatal("should acquire lock")
	}
	if lock.Acquire() {
		t.Fatal("should NOT re-acquire")
	}
	lock.Release()
	if !lock.Acquire() {
		t.Fatal("should acquire after release")
	}
	lock.ForceRelease()
}

func TestLockBlock(t *testing.T) {
	s := newTestStore()
	lock := s.Lock("res", time.Second)
	lock.Acquire()

	go func() {
		time.Sleep(50 * time.Millisecond)
		lock.Release()
	}()

	executed := false
	ok := lock.Block(200*time.Millisecond, func() { executed = true })
	if !ok || !executed {
		t.Fatal("Block should succeed after release")
	}
}

func TestConcurrentSafety(t *testing.T) {
	s := newTestStore()
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := "k"
			_ = s.Put(key, i, time.Minute)
			_ = s.Get(key)
			_, _ = s.Increment("counter")
			_ = s.HSet("h", key, i)
			_, _ = s.HGet("h", key)
			s.Tags("t").Put(key, i, time.Minute)
		}(i)
	}
	wg.Wait()
	// 只要不 panic / deadlock 即为通过
}

// TestNewCacheManager_RedisFallbackToFile 验证 redis 连接失败时自动降级为文件缓存，
// 而不是返回错误（客户场景：redis 抖动不应导致应用启动失败）。
func TestNewCacheManager_RedisFallbackToFile(t *testing.T) {
	cfg, err := config.NewConfig("/nonexistent/cache-test.yaml")
	if err != nil {
		t.Fatalf("构造空配置失败: %v", err)
	}
	cfg.Set("cache.driver", "redis")
	cfg.Set("cache.redis.host", "127.0.0.1")
	cfg.Set("cache.redis.port", 1) // 必然被拒绝的端口，确保连接失败
	cfg.Set("cache.file.path", t.TempDir())

	m, err := NewCacheManager(cfg)
	if err != nil {
		t.Fatalf("redis 连接失败不应导致初始化报错: %v", err)
	}
	cm := m.(*cacheManager)
	defer cm.Stop()

	if cm.defaultStore != "file" {
		t.Fatalf("redis 不可用时应降级为 file，实际 %s", cm.defaultStore)
	}
	if err := m.Put("k", "v", time.Minute); err != nil {
		t.Fatalf("降级后写入失败: %v", err)
	}
	if m.GetString("k") != "v" {
		t.Fatal("降级后读取失败")
	}
}

// TestNewCacheManager_RedisFileFallbackToMemory 验证 redis 与 file 均不可用时的兜底链。
func TestNewCacheManager_RedisFileFallbackToMemory(t *testing.T) {
	f := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.NewConfig("/nonexistent/cache-test.yaml")
	if err != nil {
		t.Fatalf("构造空配置失败: %v", err)
	}
	cfg.Set("cache.driver", "redis")
	cfg.Set("cache.redis.host", "127.0.0.1")
	cfg.Set("cache.redis.port", 1)
	cfg.Set("cache.file.path", f) // 文件缓存目录不可用

	m, err := NewCacheManager(cfg)
	if err != nil {
		t.Fatalf("应兜底为内存缓存而非报错: %v", err)
	}
	cm := m.(*cacheManager)
	defer cm.Stop()

	if cm.defaultStore != "memory" {
		t.Fatalf("redis/file 均不可用时应降级为 memory，实际 %s", cm.defaultStore)
	}
	if err := m.Put("k", "v", time.Minute); err != nil {
		t.Fatalf("降级后写入失败: %v", err)
	}
	if m.GetString("k") != "v" {
		t.Fatal("降级后读取失败")
	}
}

// TestNewCacheManager_DefaultDriverIsFile 验证未配置驱动时默认使用文件缓存。
func TestNewCacheManager_DefaultDriverIsFile(t *testing.T) {
	cfg, err := config.NewConfig("/nonexistent/cache-test.yaml")
	if err != nil {
		t.Fatalf("构造空配置失败: %v", err)
	}
	cfg.Set("cache.file.path", t.TempDir())

	m, err := NewCacheManager(cfg)
	if err != nil {
		t.Fatalf("初始化失败: %v", err)
	}
	cm := m.(*cacheManager)
	defer cm.Stop()

	if cm.defaultStore != "file" {
		t.Fatalf("未配置驱动时默认应为 file，实际 %s", cm.defaultStore)
	}
}

// TestNewCacheManager_FileDriver 验证 file 驱动经管理器正常读写与关闭。
func TestNewCacheManager_FileDriver(t *testing.T) {
	cfg, err := config.NewConfig("/nonexistent/cache-test.yaml")
	if err != nil {
		t.Fatalf("构造空配置失败: %v", err)
	}
	cfg.Set("cache.driver", "file")
	cfg.Set("cache.file.path", t.TempDir())

	m, err := NewCacheManager(cfg)
	if err != nil {
		t.Fatalf("file 驱动初始化失败: %v", err)
	}
	cm := m.(*cacheManager)
	defer cm.Stop()

	if m.Store("file") == nil || m.Store("file") != cm.defaultCacheStore() {
		t.Fatal("file store 应为默认 store")
	}
	if err := m.Put("k", "v", time.Minute); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if m.GetString("k") != "v" {
		t.Fatal("读取失败")
	}
	cm.Stop() // 可重复调用
}

// TestNewCacheManager_FileFallbackToMemory 验证文件缓存路径不可用时降级为内存缓存。
func TestNewCacheManager_FileFallbackToMemory(t *testing.T) {
	f := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.NewConfig("/nonexistent/cache-test.yaml")
	if err != nil {
		t.Fatalf("构造空配置失败: %v", err)
	}
	cfg.Set("cache.driver", "file")
	cfg.Set("cache.file.path", f)

	m, err := NewCacheManager(cfg)
	if err != nil {
		t.Fatalf("文件缓存不可用应降级而非报错: %v", err)
	}
	cm := m.(*cacheManager)
	defer cm.Stop()

	if cm.defaultStore != "memory" {
		t.Fatalf("file 不可用时应降级为 memory，实际 %s", cm.defaultStore)
	}
	if err := m.Put("k", "v", time.Minute); err != nil {
		t.Fatalf("降级后写入失败: %v", err)
	}
	if m.GetString("k") != "v" {
		t.Fatal("降级后读取失败")
	}
}
