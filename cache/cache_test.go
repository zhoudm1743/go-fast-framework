package cache

import (
	"sync"
	"testing"
	"time"
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
