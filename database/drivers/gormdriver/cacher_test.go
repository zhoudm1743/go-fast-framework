package gormdriver

import (
	"testing"
	"time"

	"github.com/zhoudm1743/go-fast-framework/cache"
	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// testCache 测试用的 contracts.Cache 实现，基于框架内存 store。
type testCache struct {
	contracts.CacheStore
}

func (c *testCache) Store(name string) contracts.CacheStore { return c.CacheStore }

func newTestCache() contracts.Cache {
	return &testCache{CacheStore: cache.NewMemoryStore(16, 0)}
}

// newTestDriverWithCaches 创建已启用查询缓存插件的测试驱动。
func newTestDriverWithCaches(t *testing.T) *GormDriver {
	t.Helper()
	drv := newTestDriverWithTable(t)
	if err := drv.EnableCaches(newTestCache()); err != nil {
		t.Fatalf("启用查询缓存失败: %v", err)
	}
	return drv
}

// TestQueryCache_HitAndMiss 验证 Cache() 按需缓存与命中行为。
func TestQueryCache_HitAndMiss(t *testing.T) {
	drv := newTestDriverWithCaches(t)
	q := drv.Query()

	if err := q.Create(&TestModel{ID: "c001", Name: "alice"}); err != nil {
		t.Fatalf("插入失败: %v", err)
	}

	// 第一次 Cache() 查询：走数据库，写入缓存
	var m1 TestModel
	if err := q.Cache().First(&m1, "id = ?", "c001"); err != nil {
		t.Fatalf("首次缓存查询失败: %v", err)
	}
	if m1.Name != "alice" {
		t.Fatalf("首次查询结果错误: %v", m1.Name)
	}

	// 用原生 SQL 直接改库（绕过 caches 插件的失效回调），模拟数据被外部修改
	if err := q.Exec("UPDATE test_models SET name = 'bob' WHERE id = ?", "c001"); err != nil {
		t.Fatalf("直接改库失败: %v", err)
	}

	// 第二次 Cache() 查询：应命中缓存，返回旧值 alice
	var m2 TestModel
	if err := q.Cache().First(&m2, "id = ?", "c001"); err != nil {
		t.Fatalf("命中缓存查询失败: %v", err)
	}
	if m2.Name != "alice" {
		t.Fatalf("缓存命中失败，期望 alice 实际 %v", m2.Name)
	}

	// 未调用 Cache() 的查询：不走缓存，应返回新值 bob
	var m3 TestModel
	if err := q.First(&m3, "id = ?", "c001"); err != nil {
		t.Fatalf("非缓存查询失败: %v", err)
	}
	if m3.Name != "bob" {
		t.Fatalf("非缓存查询应返回最新值，期望 bob 实际 %v", m3.Name)
	}
}

// TestQueryCache_InvalidateOnWrite 验证写操作自动失效缓存。
func TestQueryCache_InvalidateOnWrite(t *testing.T) {
	drv := newTestDriverWithCaches(t)
	q := drv.Query()

	if err := q.Create(&TestModel{ID: "c002", Name: "alice"}); err != nil {
		t.Fatalf("插入失败: %v", err)
	}

	var m1 TestModel
	if err := q.Cache().First(&m1, "id = ?", "c002"); err != nil {
		t.Fatalf("首次缓存查询失败: %v", err)
	}
	if m1.Name != "alice" {
		t.Fatalf("首次查询结果错误: %v", m1.Name)
	}

	// 写操作（Update）应触发 Invalidate，清空查询缓存
	if err := q.Model(&TestModel{}).Where("id = ?", "c002").Update("name", "bob"); err != nil {
		t.Fatalf("更新失败: %v", err)
	}

	// 再次 Cache() 查询：缓存已失效，应走数据库返回新值 bob
	var m2 TestModel
	if err := q.Cache().First(&m2, "id = ?", "c002"); err != nil {
		t.Fatalf("失效后缓存查询失败: %v", err)
	}
	if m2.Name != "bob" {
		t.Fatalf("写操作应使缓存失效，期望 bob 实际 %v", m2.Name)
	}
}

// TestQueryCache_CustomTTLAndTag 验证 TTL 与自定义标签选项。
func TestQueryCache_CustomTTLAndTag(t *testing.T) {
	drv := newTestDriverWithCaches(t)
	q := drv.Query()

	if err := q.Create(&TestModel{ID: "c003", Name: "carol"}); err != nil {
		t.Fatalf("插入失败: %v", err)
	}

	// 使用自定义 TTL 与标签
	var m1 TestModel
	err := q.Cache(
		contracts.CacheTTLOption(time.Minute),
		contracts.CacheTagsOption("users"),
	).First(&m1, "id = ?", "c003")
	if err != nil {
		t.Fatalf("带选项的缓存查询失败: %v", err)
	}
	if m1.Name != "carol" {
		t.Fatalf("查询结果错误: %v", m1.Name)
	}

	// 命中自定义标签对应的缓存
	var m2 TestModel
	if err := q.Cache(contracts.CacheTagsOption("users")).First(&m2, "id = ?", "c003"); err != nil {
		t.Fatalf("再次缓存查询失败: %v", err)
	}
	if m2.Name != "carol" {
		t.Fatalf("缓存命中结果错误: %v", m2.Name)
	}
}

// TestQueryCache_DisabledPluginNoOp 验证未启用插件时 Cache() 无副作用。
func TestQueryCache_DisabledPluginNoOp(t *testing.T) {
	drv := newTestDriverWithTable(t) // 未启用缓存插件
	q := drv.Query()

	if err := q.Create(&TestModel{ID: "c004", Name: "dave"}); err != nil {
		t.Fatalf("插入失败: %v", err)
	}

	// 调用 Cache() 不应报错，查询正常返回
	var m1 TestModel
	if err := q.Cache().First(&m1, "id = ?", "c004"); err != nil {
		t.Fatalf("未启用插件时 Cache() 查询失败: %v", err)
	}
	if m1.Name != "dave" {
		t.Fatalf("查询结果错误: %v", m1.Name)
	}
}
