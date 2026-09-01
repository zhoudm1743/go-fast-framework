package mock

import (
	"testing"

	"github.com/zhoudm1743/go-fast-framework/foundation"
)

func TestManagerSwapAndRestore(t *testing.T) {
	app := foundation.NewApplication(".")
	app.Singleton("cache", func(app foundation.Application) (any, error) {
		return NewMockCache(), nil
	})

	m := NewManager(app)
	original, _ := app.Make("cache")

	newCache := NewMockCache()
	newCache.Put("key", "value", 0)
	m.Swap("cache", newCache)

	if !app.Bound("cache") {
		t.Fatal("cache should still be bound after swap")
	}
	if c := app.MustMake("cache").(*MockCache); c.Get("key") != "value" {
		t.Fatal("swapped cache should return mocked value")
	}

	m.Restore("cache")
	if app.MustMake("cache") != original {
		t.Fatal("cache should be restored to original instance")
	}
}

func TestManagerRestoreUnbound(t *testing.T) {
	app := foundation.NewApplication(".")
	m := NewManager(app)

	m.Swap("cache", NewMockCache())
	if !app.Bound("cache") {
		t.Fatal("cache should be bound after swap")
	}

	m.Restore("cache")
	if app.Bound("cache") {
		t.Fatal("cache should be unbound after restore")
	}
}

func TestManagerRestoreAll(t *testing.T) {
	app := foundation.NewApplication(".")
	app.Singleton("cache", func(app foundation.Application) (any, error) {
		return NewMockCache(), nil
	})

	m := NewManager(app)
	m.Swap("cache", NewMockCache())
	m.Swap("log", NewMockLog())

	m.RestoreAll()

	if !app.Bound("cache") {
		t.Fatal("cache should be restored")
	}
	if app.Bound("log") {
		t.Fatal("log should be unbound because it was not originally bound")
	}
}

func TestMockCache(t *testing.T) {
	c := NewMockCache()
	_ = c.Put("name", "go-fast", 0)
	if c.GetString("name") != "go-fast" {
		t.Fatalf("expected go-fast, got %v", c.Get("name"))
	}
	if !c.Has("name") {
		t.Fatal("expected key to exist")
	}
	_ = c.Forget("name")
	if c.Has("name") {
		t.Fatal("expected key to be forgotten")
	}
}

func TestMockLog(t *testing.T) {
	l := NewMockLog()
	l.Info("hello")
	if len(l.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(l.Entries))
	}
	if l.Entries[0].Level != "info" {
		t.Fatalf("expected info level, got %s", l.Entries[0].Level)
	}
}
