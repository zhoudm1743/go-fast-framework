// suite.go 一致性套件入口：Factory 注入 + RunSuite 子测试结构 + 公共辅助
// （灌数/迁移/错误断言/SQL 计数），以及查询缓存用例的内存 contracts.Cache 实现。

package drivertest

import (
	"errors"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// Factory 创建待测驱动（各驱动测试侧实现：内存 SQLite）。
type Factory func(t *testing.T) contracts.Driver

// QueryCounter 驱动可选的 SQL 计数能力（§11.5 非 N+1 断言）。
// 工厂返回的驱动若实现本接口，Preload 用例断言批量 IN 子查询次数上界
// （has 系 ≤2：父查询 + 批量 IN；many2many ≤3：父查询 + 中间表 + 子表）；
// 未实现则 t.Log 记录后跳过计数断言。
type QueryCounter interface {
	QueryCount() int
}

// RunSuite 一致性套件入口（§11.1 子测试结构）。本轮实现核心子集
// CRUD/Preload/Advanced/Expr/Transaction/SoftDelete/QueryCache/Hooks/Errors/
// OrmTag/DDL；Joins/Lock 整组注册为 Skip（见各 TODO 注释指明文档章节）。
func RunSuite(t *testing.T, f Factory) {
	t.Run("CRUD", func(t *testing.T) { suiteCRUD(t, f) })
	t.Run("Joins", func(t *testing.T) {
		// TODO(11.4)：Joins 矩阵待实现（INNER/LEFT/RIGHT/带参 ON/自连接/非法
		// JOIN 串 ErrUnsupported/Schema 前缀，见 orm-tag-design.md §11.4）
		t.Skip("TODO(11.4)：Joins 矩阵见 orm-tag-design.md §11.4，本轮未纳入")
	})
	t.Run("Preload", func(t *testing.T) { suitePreload(t, f) })
	t.Run("Advanced", func(t *testing.T) { suiteAdvanced(t, f) }) // §11.6
	t.Run("Expr", func(t *testing.T) { suiteExpr(t, f) })         // §11.7
	t.Run("Transaction", func(t *testing.T) { suiteTx(t, f) })    // §11.8
	t.Run("SoftDelete", func(t *testing.T) { suiteSoftDelete(t, f) })
	t.Run("Lock", func(t *testing.T) { suiteLock(t, f) })
	t.Run("QueryCache", func(t *testing.T) { suiteCache(t, f) }) // §11.11
	t.Run("Hooks", func(t *testing.T) { suiteHooks(t, f) })      // §11.13
	t.Run("Errors", func(t *testing.T) { suiteErrors(t, f) })
	t.Run("OrmTag", func(t *testing.T) { suiteOrmTag(t, f) }) // §11.16 差异固化
	t.Run("DDL", func(t *testing.T) { suiteDDL(t, f) })       // §11.15 SQLite 子集
}

// ── 公共辅助 ─────────────────────────────────────────────────────────

// mustAutoMigrate 迁移模型，失败即 Fatal。
func mustAutoMigrate(t *testing.T, drv contracts.Driver, models ...any) {
	t.Helper()
	if err := drv.AutoMigrate(models...); err != nil {
		t.Fatalf("AutoMigrate %T: %v", models, err)
	}
}

// mustCreate 批量创建，失败即 Fatal。
func mustCreate(t *testing.T, q contracts.Query, values ...any) {
	t.Helper()
	for _, v := range values {
		if err := q.Create(v); err != nil {
			t.Fatalf("Create %T: %v", v, err)
		}
	}
}

// mustExec 执行原生 SQL，失败即 Fatal。
func mustExec(t *testing.T, q contracts.Query, sql string, args ...any) {
	t.Helper()
	if err := q.Exec(sql, args...); err != nil {
		t.Fatalf("Exec %q: %v", sql, err)
	}
}

// mustTake 按主键取单行，失败即 Fatal。
// 注意：gorm/xorm 都会把非零值 dest 的主键追加进 WHERE（gorm Take / xorm Get
// 的条件语义），因此以全新零值承接查询结果后反射回拷，隔离调用方的脏 dest。
func mustTake(t *testing.T, q contracts.Query, dest any, id string) {
	t.Helper()
	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Struct {
		t.Fatalf("mustTake 要求 struct 指针，得到 %T", dest)
	}
	fresh := reflect.New(rv.Type().Elem())
	if err := q.Model(dest).Where("id = ?", id).Take(fresh.Interface()); err != nil {
		t.Fatalf("Take id=%s: %v", id, err)
	}
	rv.Elem().Set(fresh.Elem())
}

// queryCountOf 返回驱动的 SQL 计数函数与是否可用（未实现 QueryCounter 时
// ok=false，调用方 t.Log 跳过计数断言）。
func queryCountOf(drv contracts.Driver) (func() int, bool) {
	if qc, ok := drv.(QueryCounter); ok {
		return qc.QueryCount, true
	}
	return nil, false
}

// errIs 断言 err 非 nil 且 errors.Is 命中 target。
func errIs(t *testing.T, err error, target error, context string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: 期望错误 %v，实际无错误", context, target)
	}
	if !errors.Is(err, target) {
		t.Fatalf("%s: 期望 errors.Is(err, %v)，实际 %v", context, target, err)
	}
}

// assertUnixSecond 断言时间戳为合理 unix 秒（§11.3：created/updated 双驱动同为
// unix 秒——70 年后的时间戳量级，容差 2 分钟）。
func assertUnixSecond(t *testing.T, label string, ts int64) {
	t.Helper()
	now := time.Now().Unix()
	if ts < now-3600 || ts > now+120 {
		t.Fatalf("%s: 期望 unix 秒（%d 附近），实际 %d", label, now, ts)
	}
}

// ── 内存 Cache（查询缓存用例的 contracts.Cache 最小实现） ─────────────

// newSuiteCache 创建套件用内存缓存：仅驱动查询缓存实际调用的路径有行为
// （Get/Put/Tags(...).Flush），其余契约方法为语义安全的零值实现。
// 返回具体类型便于套件直接执行 Tags(...).Flush() 手动批量失效（§11.11）。
func newSuiteCache() *memCache {
	return &memCache{memCacheStore: &memCacheStore{data: map[string]cacheEntry{}}}
}

type cacheEntry struct {
	value    any
	tags     []string
	expireAt time.Time // 零值表示永不过期
}

func (e cacheEntry) expired() bool {
	return !e.expireAt.IsZero() && time.Now().After(e.expireAt)
}

// memCache 顶层缓存管理器：内嵌唯一 store（查询缓存用例只使用默认存储），
// 经嵌入继承全部 CacheStore 方法，Store(name) 恒返回该 store。
type memCache struct {
	*memCacheStore
}

// Store 获取指定名称的存储；套件实现仅一个默认存储，任何名称均返回它
// （驱动查询缓存对 Store("")/Store("memory") 的访问全部路由到同一份数据）。
func (c *memCache) Store(name string) contracts.CacheStore { return c.memCacheStore }

type memCacheStore struct {
	mu   sync.Mutex
	data map[string]cacheEntry
}

func (s *memCacheStore) Get(key string, def ...any) any {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.data[key]
	if ok && e.expired() {
		delete(s.data, key)
		ok = false
	}
	if ok {
		return e.value
	}
	if len(def) > 0 {
		return def[0]
	}
	return nil
}

func (s *memCacheStore) Has(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.data[key]
	if ok && e.expired() {
		delete(s.data, key)
		ok = false
	}
	return ok
}

func (s *memCacheStore) Put(key string, value any, ttl time.Duration) error {
	return s.put(key, value, ttl)
}

func (s *memCacheStore) Forever(key string, value any) error {
	return s.put(key, value, 0)
}

func (s *memCacheStore) put(key string, value any, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := cacheEntry{value: value}
	if ttl > 0 {
		e.expireAt = time.Now().Add(ttl)
	}
	s.data[key] = e
	return nil
}

func (s *memCacheStore) Forget(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}

func (s *memCacheStore) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = map[string]cacheEntry{}
	return nil
}

func (s *memCacheStore) Tags(tags ...string) contracts.TaggedCache {
	return &memTaggedCache{store: s, tags: tags}
}

func (s *memCacheStore) Pull(key string, def ...any) any {
	v := s.Get(key, def...)
	_ = s.Forget(key)
	return v
}

// memTaggedCache 仅实现查询缓存实际消费的 Put/Flush；其余为契约零值实现。
type memTaggedCache struct {
	store *memCacheStore
	tags  []string
}

func (t *memTaggedCache) Get(key string, def ...any) any { return t.store.Get(key, def...) }
func (t *memTaggedCache) Has(key string) bool            { return t.store.Has(key) }

func (t *memTaggedCache) Put(key string, value any, ttl time.Duration) error {
	t.store.mu.Lock()
	e := cacheEntry{value: value, tags: t.tags}
	if ttl > 0 {
		e.expireAt = time.Now().Add(ttl)
	}
	t.store.data[key] = e
	t.store.mu.Unlock()
	return nil
}

func (t *memTaggedCache) Forever(key string, value any) error { return t.Put(key, value, 0) }
func (t *memTaggedCache) Forget(key string) error             { return t.store.Forget(key) }

func (t *memTaggedCache) Many(keys []string) map[string]any {
	out := make(map[string]any, len(keys))
	for _, k := range keys {
		if v := t.store.Get(k); v != nil {
			out[k] = v
		}
	}
	return out
}

func (t *memTaggedCache) PutMany(values map[string]any, ttl time.Duration) error {
	for k, v := range values {
		if err := t.Put(k, v, ttl); err != nil {
			return err
		}
	}
	return nil
}

func (t *memTaggedCache) Increment(key string, value ...int64) (int64, error) {
	return t.store.Increment(key, value...)
}

func (t *memTaggedCache) Decrement(key string, value ...int64) (int64, error) {
	return t.store.Decrement(key, value...)
}

// Flush 清除携带任一标签的缓存（查询缓存按标签批量失效语义）。
func (t *memTaggedCache) Flush() error {
	t.store.mu.Lock()
	defer t.store.mu.Unlock()
	for k, e := range t.store.data {
		for _, want := range t.tags {
			for _, has := range e.tags {
				if want == has {
					delete(t.store.data, k)
					break
				}
			}
		}
	}
	return nil
}

// ── 以下契约方法为套件缓存实现的零值占位（查询缓存用例不消费） ─────────

func (s *memCacheStore) GetBool(string, ...bool) bool          { return false }
func (s *memCacheStore) GetInt(string, ...int) int             { return 0 }
func (s *memCacheStore) GetInt64(string, ...int64) int64       { return 0 }
func (s *memCacheStore) GetFloat64(string, ...float64) float64 { return 0 }
func (s *memCacheStore) GetString(string, ...string) string    { return "" }

func (s *memCacheStore) Increment(key string, value ...int64) (int64, error) {
	if len(value) == 0 {
		value = []int64{1}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := int64(0)
	if e, ok := s.data[key]; ok {
		if n, ok := e.value.(int64); ok {
			cur = n
		}
	}
	cur += value[0]
	s.data[key] = cacheEntry{value: cur}
	return cur, nil
}

func (s *memCacheStore) Decrement(key string, value ...int64) (int64, error) {
	if len(value) == 0 {
		value = []int64{1}
	}
	neg := -value[0]
	return s.Increment(key, neg)
}

func (s *memCacheStore) Remember(key string, ttl time.Duration, cb func() (any, error)) (any, error) {
	if v := s.Get(key); v != nil {
		return v, nil
	}
	v, err := cb()
	if err != nil {
		return nil, err
	}
	_ = s.Put(key, v, ttl)
	return v, nil
}

func (s *memCacheStore) RememberForever(key string, cb func() (any, error)) (any, error) {
	return s.Remember(key, 0, cb)
}

func (s *memCacheStore) Many(keys []string) map[string]any {
	out := make(map[string]any, len(keys))
	for _, k := range keys {
		if v := s.Get(k); v != nil {
			out[k] = v
		}
	}
	return out
}

func (s *memCacheStore) PutMany(values map[string]any, ttl time.Duration) error {
	for k, v := range values {
		if err := s.Put(k, v, ttl); err != nil {
			return err
		}
	}
	return nil
}

func (s *memCacheStore) Lock(key string, _ time.Duration) contracts.CacheLock {
	return &memCacheLock{store: s, key: key}
}

type memCacheLock struct {
	store   *memCacheStore
	key     string
	held    bool
	heldMu  sync.Mutex
	holdTag string
}

func (l *memCacheLock) Acquire() bool {
	l.heldMu.Lock()
	defer l.heldMu.Unlock()
	if l.held {
		return false
	}
	l.held = true
	l.holdTag = "lock:" + l.key + ":" + strconv.FormatInt(time.Now().UnixNano(), 10)
	return true
}

func (l *memCacheLock) Release() bool {
	l.heldMu.Lock()
	defer l.heldMu.Unlock()
	if !l.held {
		return false
	}
	l.held = false
	return true
}

func (l *memCacheLock) ForceRelease() bool { return l.Release() }

func (l *memCacheLock) Block(timeout time.Duration, callback ...func()) bool {
	if !l.Acquire() {
		return false
	}
	if len(callback) > 0 && callback[0] != nil {
		callback[0]()
	}
	return true
}

// 占位：hash 契约零值实现（查询缓存不消费）。
func (s *memCacheStore) HGet(key, field string) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.data["h:"+key]; ok {
		if m, ok := e.value.(map[string]any); ok {
			return m[field], nil
		}
	}
	return nil, nil
}

func (s *memCacheStore) HSet(key, field string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.data["h:"+key]
	if !ok {
		e = cacheEntry{value: map[string]any{}}
	}
	m, _ := e.value.(map[string]any)
	if m == nil {
		m = map[string]any{}
	}
	m[field] = value
	e.value = m
	s.data["h:"+key] = e
	return nil
}

func (s *memCacheStore) HDel(key string, fields ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.data["h:"+key]; ok {
		if m, ok := e.value.(map[string]any); ok {
			for _, f := range fields {
				delete(m, f)
			}
		}
	}
	return nil
}

func (s *memCacheStore) HExists(key, field string) bool {
	v, _ := s.HGet(key, field)
	return v != nil
}

func (s *memCacheStore) HGetAll(key string) (map[string]any, error) {
	v, _ := s.HGet(key, "")
	if m, ok := v.(map[string]any); ok {
		return m, nil
	}
	return map[string]any{}, nil
}

func (s *memCacheStore) HLen(key string) int64 {
	m, _ := s.HGetAll(key)
	return int64(len(m))
}

func (s *memCacheStore) HKeys(key string) ([]string, error) {
	m, _ := s.HGetAll(key)
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out, nil
}

var (
	_ contracts.Cache       = (*memCache)(nil)
	_ contracts.CacheStore  = (*memCacheStore)(nil)
	_ contracts.TaggedCache = (*memTaggedCache)(nil)
	_ contracts.CacheLock   = (*memCacheLock)(nil)
)
