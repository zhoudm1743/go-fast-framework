package ratelimit

import (
	"sync"
	"testing"
	"time"

	"github.com/zhoudm1743/go-fast-framework/cache"
	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// memCache 将真实内存 store 包装成 contracts.Cache（追加 Store 方法），用于 TTL 与并发测试。
type memCache struct {
	contracts.CacheStore
}

func (m *memCache) Store(string) contracts.CacheStore { return m }

func newMemCache() contracts.Cache {
	return &memCache{CacheStore: cache.NewMemoryStore(16, 0)}
}

func TestAllow_FixedWindow(t *testing.T) {
	l := New(newMemCache(), "")

	for i := 0; i < 3; i++ {
		allowed, remaining := l.Allow("login", 3, time.Minute)
		if !allowed {
			t.Fatalf("hit %d should be allowed", i+1)
		}
		if want := int64(3 - i - 1); remaining != want {
			t.Fatalf("hit %d: remaining = %d, want %d", i+1, remaining, want)
		}
	}

	allowed, remaining := l.Allow("login", 3, time.Minute)
	if allowed {
		t.Fatal("4th hit should be denied")
	}
	if remaining != 0 {
		t.Fatalf("remaining = %d, want 0", remaining)
	}
}

func TestHit_Counts(t *testing.T) {
	l := New(newMemCache(), "")
	for i := int64(1); i <= 4; i++ {
		count, err := l.Hit("k", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if count != i {
			t.Fatalf("count = %d, want %d", count, i)
		}
	}
}

func TestTooManyAttempts_ReadOnly(t *testing.T) {
	l := New(newMemCache(), "")
	if l.TooManyAttempts("k", 2, time.Minute) {
		t.Fatal("empty key must not be too many")
	}
	_, _ = l.Hit("k", time.Minute)
	if l.TooManyAttempts("k", 2, time.Minute) {
		t.Fatal("1 attempt < limit 2 must not be too many")
	}
	_, _ = l.Hit("k", time.Minute)
	if !l.TooManyAttempts("k", 2, time.Minute) {
		t.Fatal("2 attempts >= limit 2 must be too many")
	}
	// 只读不增
	if l.Attempts("k") != 2 {
		t.Fatalf("Attempts = %d, want 2", l.Attempts("k"))
	}
}

func TestAttempts(t *testing.T) {
	l := New(newMemCache(), "")
	if n := l.Attempts("k"); n != 0 {
		t.Fatalf("empty Attempts = %d, want 0", n)
	}
	_, _ = l.Hit("k", time.Minute)
	_, _ = l.Hit("k", time.Minute)
	if n := l.Attempts("k"); n != 2 {
		t.Fatalf("Attempts = %d, want 2", n)
	}
}

func TestAvailableIn(t *testing.T) {
	l := New(newMemCache(), "")
	if d := l.AvailableIn("k"); d != 0 {
		t.Fatalf("empty AvailableIn = %v, want 0", d)
	}

	window := 2 * time.Second
	_, _ = l.Hit("k", window)
	if d := l.AvailableIn("k"); d <= 0 || d > window {
		t.Fatalf("AvailableIn = %v, want (0, %v]", d, window)
	}
}

func TestClear(t *testing.T) {
	ms := cache.NewMemoryStore(16, 0)
	l := New(&memCache{CacheStore: ms}, "")
	_, _ = l.Hit("k", time.Minute)
	if l.Attempts("k") != 1 {
		t.Fatal("setup failed")
	}

	if err := l.Clear("k"); err != nil {
		t.Fatal(err)
	}
	if l.Attempts("k") != 0 {
		t.Fatalf("Attempts after Clear = %d, want 0", l.Attempts("k"))
	}
	if l.AvailableIn("k") != 0 {
		t.Fatalf("AvailableIn after Clear = %v, want 0", l.AvailableIn("k"))
	}
	if ms.Has("ratelimit:k") || ms.Has("ratelimit:k:timer") {
		t.Fatal("count/timer keys must be removed after Clear")
	}
}

func TestKeyPrefix(t *testing.T) {
	ms := cache.NewMemoryStore(16, 0)
	l := New(&memCache{CacheStore: ms}, "")
	_, _ = l.Hit("login", time.Minute)
	if !ms.Has("ratelimit:login") {
		t.Fatal("count key missing")
	}
	if !ms.Has("ratelimit:login:timer") {
		t.Fatal("timer key missing")
	}
}

func TestWindowExpiryResets(t *testing.T) {
	l := New(newMemCache(), "")
	window := 50 * time.Millisecond

	allowed, _ := l.Allow("k", 1, window)
	if !allowed {
		t.Fatal("first hit should be allowed")
	}
	if allowed, _ = l.Allow("k", 1, window); allowed {
		t.Fatal("second hit within window should be denied")
	}

	time.Sleep(80 * time.Millisecond)
	if allowed, _ = l.Allow("k", 1, window); !allowed {
		t.Fatal("after window expiry, hit should be allowed again")
	}
}

func TestConcurrentHit(t *testing.T) {
	l := New(newMemCache(), "")
	// 预热：先建立计数键，随后并发自增走 Increment 原子路径
	if _, err := l.Hit("k", time.Minute); err != nil {
		t.Fatal(err)
	}

	const n = 200
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if _, err := l.Hit("k", time.Minute); err != nil {
				t.Errorf("Hit() error: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := l.Attempts("k"); got != 1+n {
		t.Fatalf("Attempts = %d, want %d", got, 1+n)
	}
}

var _ contracts.RateLimiter = (*rateLimiter)(nil)
