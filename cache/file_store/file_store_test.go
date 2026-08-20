package fileStore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

func newTestStore(t *testing.T) *FileStore {
	t.Helper()
	s, err := New(t.TempDir(), 0)
	if err != nil {
		t.Fatalf("创建测试 store 失败: %v", err)
	}
	return s
}

func newTestStoreDir(t *testing.T, dir string) *FileStore {
	t.Helper()
	s, err := New(dir, 0)
	if err != nil {
		t.Fatalf("创建测试 store 失败: %v", err)
	}
	return s
}

// ── 基础 CRUD ─────────────────────────────────────────────────────────

func TestPutGet_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	cases := []struct {
		name string
		key  string
		want any
	}{
		{"string", "k_str", "hello"},
		{"int", "k_int", 42},
		{"int64", "k_i64", int64(1 << 40)},
		{"float", "k_f", 3.14},
		{"bool", "k_b", true},
		{"map", "k_map", map[string]any{"a": 1, "b": "x"}},
		{"nil", "k_nil", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := s.Put(c.key, c.want, time.Minute); err != nil {
				t.Fatalf("Put 失败: %v", err)
			}
			got := s.Get(c.key)
			// nil 与 map 用 JSON 语义对比（数字统一为 float64）
			wantJSON, _ := json.Marshal(c.want)
			gotJSON, _ := json.Marshal(got)
			if !bytes.Equal(wantJSON, gotJSON) {
				t.Fatalf("Get(%q) = %v, want %v", c.key, got, c.want)
			}
		})
	}
}

func TestPutGet_BytesBinary(t *testing.T) {
	s := newTestStore(t)
	orig := []byte{0xff, 0x00, 0x80, 0x7f, '"', '\\', '\n', 0xfe, '{', '}'}
	if err := s.Put("bin", orig, time.Minute); err != nil {
		t.Fatalf("Put 失败: %v", err)
	}
	got, ok := s.Get("bin").([]byte)
	if !ok {
		t.Fatalf("Get 应还原为 []byte，实际 %T", s.Get("bin"))
	}
	if !bytes.Equal(got, orig) {
		t.Fatalf("二进制字节往返不一致: %v vs %v", got, orig)
	}
}

func TestPutGet_EmptyBytes(t *testing.T) {
	s := newTestStore(t)
	if err := s.Put("empty", []byte{}, time.Minute); err != nil {
		t.Fatalf("Put 失败: %v", err)
	}
	got, ok := s.Get("empty").([]byte)
	if !ok {
		t.Fatalf("空字节应还原为 []byte，实际 %T", s.Get("empty"))
	}
	if len(got) != 0 {
		t.Fatalf("空字节长度应为 0，实际 %d", len(got))
	}
}

func TestGet_Typed(t *testing.T) {
	s := newTestStore(t)
	_ = s.Put("i", 42, 0)
	_ = s.Put("s", "hi", 0)
	_ = s.Put("b", true, 0)
	_ = s.Put("f", 2.5, 0)

	if s.GetInt("i") != 42 || s.GetInt64("i") != 42 {
		t.Fatal("GetInt/GetInt64 错误")
	}
	if s.GetString("s") != "hi" {
		t.Fatal("GetString 错误")
	}
	if !s.GetBool("b") {
		t.Fatal("GetBool 错误")
	}
	if s.GetFloat64("f") != 2.5 {
		t.Fatal("GetFloat64 错误")
	}
	// 默认值
	if s.GetString("missing", "def") != "def" || s.GetInt("missing", 7) != 7 {
		t.Fatal("默认值逻辑错误")
	}
}

func TestExpiry(t *testing.T) {
	s := newTestStore(t)
	_ = s.Put("k", "v", 50*time.Millisecond)
	if !s.Has("k") {
		t.Fatal("未过期前应存在")
	}
	time.Sleep(100 * time.Millisecond)
	if s.Has("k") {
		t.Fatal("过期后 Has 应为 false")
	}
	if s.Get("k", "def") != "def" {
		t.Fatal("过期后 Get 应返回默认值")
	}
}

// TestExpiredFileRemovedLazily 验证惰性清理：读取过期 key 时文件被物理删除。
func TestExpiredFileRemovedLazily(t *testing.T) {
	s := newTestStore(t)
	_ = s.Put("k", "v", 30*time.Millisecond)
	path := s.filePath("k")
	time.Sleep(80 * time.Millisecond)
	_ = s.Get("k")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("过期文件应在读取时被删除")
	}
}

func TestForever(t *testing.T) {
	s := newTestStore(t)
	_ = s.Forever("k", "v")
	if s.GetString("k") != "v" {
		t.Fatal("Forever 值错误")
	}
	time.Sleep(30 * time.Millisecond)
	if !s.Has("k") {
		t.Fatal("Forever 不应过期")
	}
}

func TestPull(t *testing.T) {
	s := newTestStore(t)
	_ = s.Put("k", "v", 0)
	if v := s.Pull("k"); v != "v" {
		t.Fatalf("Pull 值错误: %v", v)
	}
	if s.Has("k") {
		t.Fatal("Pull 后应删除")
	}
}

func TestForget_NonExistentNoError(t *testing.T) {
	s := newTestStore(t)
	if err := s.Forget("never-existed"); err != nil {
		t.Fatalf("删除不存在的 key 不应报错: %v", err)
	}
}

func TestFlush_RecreatesDir(t *testing.T) {
	s := newTestStore(t)
	_ = s.Put("a", 1, 0)
	_ = s.HSet("h", "f", 2)
	if err := s.Flush(); err != nil {
		t.Fatalf("Flush 失败: %v", err)
	}
	if s.Has("a") || s.HExists("h", "f") {
		t.Fatal("Flush 后数据应清空")
	}
	if info, err := os.Stat(s.dir); err != nil || !info.IsDir() {
		t.Fatal("Flush 后目录应重建")
	}
	// Flush 后可继续写入
	if err := s.Put("b", 3, 0); err != nil || s.GetInt("b") != 3 {
		t.Fatal("Flush 后写入失败")
	}
}

// ── 原子操作 ─────────────────────────────────────────────────────────

func TestIncrementDecrement(t *testing.T) {
	s := newTestStore(t)
	n, _ := s.Increment("c")
	if n != 1 {
		t.Fatalf("期望 1，实际 %d", n)
	}
	n, _ = s.Increment("c", 5)
	if n != 6 {
		t.Fatalf("期望 6，实际 %d", n)
	}
	n, _ = s.Decrement("c", 2)
	if n != 4 {
		t.Fatalf("期望 4，实际 %d", n)
	}
	if s.GetInt64("c") != 4 {
		t.Fatal("落盘值与返回值不一致")
	}
}

func TestIncrement_NonNumericError(t *testing.T) {
	s := newTestStore(t)
	_ = s.Put("s", "not-a-number", 0)
	if _, err := s.Increment("s"); err == nil {
		t.Fatal("非数值自增应报错")
	}
	if s.GetString("s") != "not-a-number" {
		t.Fatal("报错后原值不应被破坏")
	}
}

// TestIncrement_PreservesTTL 验证自增保留原有过期时间（与 memory store 一致）。
func TestIncrement_PreservesTTL(t *testing.T) {
	s := newTestStore(t)
	_ = s.Put("c", 10, 80*time.Millisecond)
	_, _ = s.Increment("c")
	e, found, err := s.readFile(s.filePath("c"))
	if err != nil || !found {
		t.Fatalf("读文件失败: found=%v err=%v", found, err)
	}
	if e.expireAt <= time.Now().UnixNano() {
		t.Fatal("自增后应保留原过期时间")
	}
	time.Sleep(130 * time.Millisecond)
	if s.Has("c") {
		t.Fatal("自增后过期时间应仍生效")
	}
}

func TestRemember_CallbackOnce(t *testing.T) {
	s := newTestStore(t)
	calls := 0
	cb := func() (any, error) { calls++; return "computed", nil }

	v1, _ := s.Remember("r", time.Minute, cb)
	v2, _ := s.Remember("r", time.Minute, cb)
	if v1 != "computed" || v2 != "computed" || calls != 1 {
		t.Fatalf("callback 应只执行一次: v1=%v v2=%v calls=%d", v1, v2, calls)
	}
}

func TestRemember_ErrorPropagation(t *testing.T) {
	s := newTestStore(t)
	cb := func() (any, error) { return nil, fmt.Errorf("boom") }
	if _, err := s.Remember("r", time.Minute, cb); err == nil || err.Error() != "boom" {
		t.Fatalf("callback 错误应原样返回: %v", err)
	}
	if s.Has("r") {
		t.Fatal("callback 失败不应写缓存")
	}
}

// ── 批量操作 ─────────────────────────────────────────────────────────

func TestManyPutMany(t *testing.T) {
	s := newTestStore(t)
	_ = s.PutMany(map[string]any{"a": 1, "b": 2}, time.Minute)
	m := s.Many([]string{"a", "b", "c"})
	if m["a"] != float64(1) || m["b"] != float64(2) || m["c"] != nil {
		t.Fatalf("Many 结果错误: %v", m)
	}
}

// ── Hash ─────────────────────────────────────────────────────────────

func TestHash_AllOps(t *testing.T) {
	s := newTestStore(t)
	_ = s.HSet("user:1", "name", "Alice")
	_ = s.HSet("user:1", "age", 30)

	if v, _ := s.HGet("user:1", "name"); v != "Alice" {
		t.Fatalf("HGet 错误: %v", v)
	}
	if !s.HExists("user:1", "age") || s.HExists("user:1", "missing") {
		t.Fatal("HExists 错误")
	}
	if s.HLen("user:1") != 2 {
		t.Fatalf("HLen 期望 2，实际 %d", s.HLen("user:1"))
	}
	keys, _ := s.HKeys("user:1")
	if len(keys) != 2 {
		t.Fatalf("HKeys 期望 2 个，实际 %d", len(keys))
	}
	all, _ := s.HGetAll("user:1")
	if all["name"] != "Alice" || all["age"] != float64(30) {
		t.Fatalf("HGetAll 错误: %v", all)
	}

	_ = s.HDel("user:1", "age")
	if s.HExists("user:1", "age") {
		t.Fatal("HDel 后字段应不存在")
	}
	if _, err := s.HGet("user:1", "missing"); err != nil {
		t.Fatalf("HGet 不存在字段应返回 nil 无错误: %v", err)
	}
}

func TestHash_FieldValueTypes(t *testing.T) {
	s := newTestStore(t)
	_ = s.HSet("h", "f", []byte{0x01, 0x02})
	_ = s.HSet("h", "n", 3.5)
	_ = s.HSet("h", "s", "str")

	got, _ := s.HGet("h", "f")
	b, ok := got.([]byte)
	if !ok || !bytes.Equal(b, []byte{0x01, 0x02}) {
		t.Fatalf("哈希字节字段保真失败: %v %T", got, got)
	}
	if v, _ := s.HGet("h", "n"); v != 3.5 {
		t.Fatalf("哈希数字字段错误: %v", v)
	}
	if v, _ := s.HGet("h", "s"); v != "str" {
		t.Fatalf("哈希字符串字段错误: %v", v)
	}
}

func TestHash_HDelAllRemovesFile(t *testing.T) {
	s := newTestStore(t)
	_ = s.HSet("h", "f", 1)
	path := s.hashFilePath("h")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("哈希文件应存在: %v", err)
	}
	_ = s.HDel("h", "f")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("清空哈希后文件应删除")
	}
}

// TestHash_ScalarIsolation 验证同名 key 的标量与哈希互不干扰。
func TestHash_ScalarIsolation(t *testing.T) {
	s := newTestStore(t)
	_ = s.Put("user:1", "scalar", 0)
	_ = s.HSet("user:1", "name", "hash")

	if s.GetString("user:1") != "scalar" {
		t.Fatal("标量值被哈希污染")
	}
	if v, _ := s.HGet("user:1", "name"); v != "hash" {
		t.Fatalf("哈希值被标量污染: %v", v)
	}
}

// ── Tags ─────────────────────────────────────────────────────────────

func TestTags_FlushIsolation(t *testing.T) {
	s := newTestStore(t)
	tagged := s.Tags("users", "api")
	_ = tagged.Put("u:1", "Alice", 0)
	_ = tagged.PutMany(map[string]any{"u:2": "Bob"}, 0)
	_, _ = tagged.Increment("cnt")
	_ = s.Put("other", "keep", 0)

	if tagged.Get("u:1") != "Alice" {
		t.Fatal("tagged Get 错误")
	}
	if err := tagged.Flush(); err != nil {
		t.Fatalf("tagged Flush 失败: %v", err)
	}
	if s.Has("u:1") || s.Has("u:2") || s.Has("cnt") {
		t.Fatal("tagged key 应全部清除")
	}
	if !s.Has("other") {
		t.Fatal("无标签 key 不应被清除")
	}
	// Flush 后 tag 组应移除，再次 Put 后可正常 Flush
	_ = tagged.Put("u:3", "Carol", 0)
	if err := tagged.Flush(); err != nil || s.Has("u:3") {
		t.Fatal("tag 重复 Flush 失败")
	}
}

// ── 锁 ────────────────────────────────────────────────────────────────

func TestLock_AcquireRelease(t *testing.T) {
	s := newTestStore(t)
	l := s.Lock("res", time.Second)
	if !l.Acquire() {
		t.Fatal("应获取锁")
	}
	if l.Acquire() {
		t.Fatal("不应重复获取")
	}
	if !l.Release() {
		t.Fatal("应释放锁")
	}
	if !l.Acquire() {
		t.Fatal("释放后应可再次获取")
	}
	l.ForceRelease()
	if !l.Acquire() {
		t.Fatal("强制释放后应可获取")
	}
}

func TestLock_Block(t *testing.T) {
	s := newTestStore(t)
	l := s.Lock("res", time.Second)
	if !l.Acquire() {
		t.Fatal("应获取锁")
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		l.Release()
	}()
	executed := false
	if !l.Block(500*time.Millisecond, func() { executed = true }) || !executed {
		t.Fatal("Block 应在释放后成功执行 callback")
	}
}

func TestLock_BlockTimeout(t *testing.T) {
	s := newTestStore(t)
	l := s.Lock("res", time.Second)
	if !l.Acquire() {
		t.Fatal("应获取锁")
	}
	defer l.ForceRelease()
	if l.Block(60 * time.Millisecond) {
		t.Fatal("超时后 Block 应失败")
	}
}

// ── 路径安全 ─────────────────────────────────────────────────────────

// TestKeyPathTraversal 验证恶意 key 不会逃逸缓存目录。
func TestKeyPathTraversal(t *testing.T) {
	dir := t.TempDir()
	s := newTestStoreDir(t, dir)

	evil := []string{"../../../etc/passwd", `..\..\windows`, "../../", "/abs/path"}
	for _, k := range evil {
		if err := s.Put(k, "safe", time.Minute); err != nil {
			t.Fatalf("Put(%q) 失败: %v", k, err)
		}
		if s.GetString(k) != "safe" {
			t.Fatalf("Get(%q) 错误", k)
		}
	}
	// 缓存目录外不应产生任何文件（比较写入前后上级目录内容）
	parent := filepath.Dir(dir)
	entries, _ := os.ReadDir(parent)
	for _, e := range entries {
		if e.Name() != filepath.Base(dir) {
			t.Fatalf("缓存目录外产生了文件: %s", e.Name())
		}
	}
}

// TestFileLayout 验证文件分片布局：{dir}/{md5前2位}/{md5后30位}.cache。
func TestFileLayout(t *testing.T) {
	s := newTestStore(t)
	_ = s.Put("layout-key", "v", 0)
	path := s.filePath("layout-key")
	rel, err := filepath.Rel(s.dir, path)
	if err != nil {
		t.Fatalf("路径不在缓存目录内: %v", err)
	}
	parts := strings.Split(rel, string(os.PathSeparator))
	if len(parts) != 2 || len(parts[0]) != 2 || !strings.HasSuffix(parts[1], cacheFileExt) {
		t.Fatalf("文件布局错误: %s", rel)
	}
}

// ── 持久化 ───────────────────────────────────────────────────────────

// TestPersistence_AcrossInstances 验证核心特性：重启（新实例）后数据仍在。
func TestPersistence_AcrossInstances(t *testing.T) {
	dir := t.TempDir()
	s1 := newTestStoreDir(t, dir)
	_ = s1.Put("p", "persisted", 0)
	_ = s1.HSet("h", "f", "hash-persisted")
	s1.Stop()

	s2 := newTestStoreDir(t, dir)
	if s2.GetString("p") != "persisted" {
		t.Fatal("重启后标量数据丢失")
	}
	if v, _ := s2.HGet("h", "f"); v != "hash-persisted" {
		t.Fatal("重启后哈希数据丢失")
	}
}

// ── 文件格式 ─────────────────────────────────────────────────────────

// TestParseFile_LegacyNoHeader 验证旧格式（无过期头）兼容读取。
func TestParseFile_LegacyNoHeader(t *testing.T) {
	s := newTestStore(t)
	legacy := `j:"legacy-value"`
	path := s.filePath("legacy")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建子目录失败: %v", err)
	}
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatalf("写旧格式文件失败: %v", err)
	}
	if s.GetString("legacy") != "legacy-value" {
		t.Fatal("旧格式数据应可读")
	}
}

// TestCorruptFile_Tolerated 验证损坏文件不 panic、可被覆盖。
func TestCorruptFile_Tolerated(t *testing.T) {
	s := newTestStore(t)
	path := s.filePath("corrupt")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not-json garbage \x00\xff"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 不 panic，返回兜底值
	_ = s.Get("corrupt")
	if err := s.Put("corrupt", "fixed", 0); err != nil {
		t.Fatalf("损坏文件应可被覆盖: %v", err)
	}
	if s.GetString("corrupt") != "fixed" {
		t.Fatal("覆盖后值错误")
	}
}

// TestHeaderExpireAt 验证头部解析与旧格式识别。
func TestHeaderExpireAt(t *testing.T) {
	s := newTestStore(t)
	past := time.Now().Add(-time.Hour).UnixNano()
	future := time.Now().Add(time.Hour).UnixNano()

	_ = s.writeFile(s.filePath("past"), past, `j:1`)
	_ = s.writeFile(s.filePath("future"), future, `j:2`)
	_ = os.WriteFile(s.filePath("legacy"), []byte(`j:3`), 0o644)

	exp, ok, _ := headerExpireAt(s.filePath("past"))
	if !ok || exp != past {
		t.Fatalf("过期头解析错误: %d %v", exp, ok)
	}
	exp, ok, _ = headerExpireAt(s.filePath("future"))
	if !ok || exp != future {
		t.Fatalf("未过期头解析错误: %d %v", exp, ok)
	}
	if _, ok, _ := headerExpireAt(s.filePath("legacy")); ok {
		t.Fatal("旧格式应返回 ok=false")
	}
}

// ── GC ───────────────────────────────────────────────────────────────

// TestGC_RemovesExpiredOnly 验证 GC 仅删除过期文件。
func TestGC_RemovesExpiredOnly(t *testing.T) {
	s := newTestStore(t)
	_ = s.Put("expired", "x", 10*time.Millisecond)
	_ = s.Forever("keep", "y")
	_ = s.HSet("h", "f", "z") // 哈希文件永不过期，GC 不应误删
	time.Sleep(50 * time.Millisecond)

	s.gc()

	if s.Has("expired") {
		t.Fatal("GC 应删除过期 key")
	}
	if !s.Has("keep") {
		t.Fatal("GC 不应删除未过期 key")
	}
	if !s.HExists("h", "f") {
		t.Fatal("GC 不应删除哈希文件")
	}
	if _, err := os.Stat(s.filePath("expired")); !os.IsNotExist(err) {
		t.Fatal("GC 后过期文件应物理删除")
	}
}

// TestGC_Background 验证后台 GC 协程按间隔清理。
func TestGC_Background(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("创建 store 失败: %v", err)
	}
	defer s.Stop()
	_ = s.Put("expired", "x", 20*time.Millisecond)
	_ = s.Forever("keep", "y")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !s.Has("expired") && s.Has("keep") {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("后台 GC 未按期清理过期文件")
}

// ── 初始化 fail-fast ─────────────────────────────────────────────────

// TestNew_FailFast_PathIsFile 验证路径为普通文件时创建失败。
func TestNew_FailFast_PathIsFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(f, 0); err == nil {
		t.Fatal("路径是文件时应报错")
	}
}

// TestNew_FailFast_EmptyPath 验证空路径报错（而非落到意外目录）。
func TestNew_FailFast_EmptyPath(t *testing.T) {
	if _, err := New("", 0); err == nil {
		t.Fatal("空路径应报错")
	}
}

// TestNew_StopIdempotent 验证 Stop 可安全重复调用。
func TestNew_StopIdempotent(t *testing.T) {
	s := newTestStore(t)
	s.Stop()
	s.Stop()
	s.Stop()
}

// ── 并发安全 ─────────────────────────────────────────────────────────

// TestConcurrent_MixedOps 混合并发操作不 panic、不损坏文件。
func TestConcurrent_MixedOps(t *testing.T) {
	s := newTestStore(t)
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("k%d", i%8)
			_ = s.Put(key, i, time.Minute)
			_ = s.Get(key)
			_, _ = s.Increment("counter")
			_ = s.HSet("h", key, i)
			_, _ = s.HGet("h", key)
			_ = s.Tags("t").Put(key, i, time.Minute)
		}(i)
	}
	wg.Wait()
	// 校验数据完整性：每个 key 都能读出合法 int
	for i := 0; i < 8; i++ {
		key := fmt.Sprintf("k%d", i)
		if _, _, err := s.readFile(s.filePath(key)); err != nil {
			t.Fatalf("并发写后文件损坏: %v", err)
		}
	}
}

// TestConcurrent_IncrementTotal 验证分片锁保证自增不丢更新。
func TestConcurrent_IncrementTotal(t *testing.T) {
	s := newTestStore(t)
	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Increment("c"); err != nil {
				t.Errorf("Increment 失败: %v", err)
			}
		}()
	}
	wg.Wait()
	if got := s.GetInt64("c"); got != n {
		t.Fatalf("并发自增期望 %d，实际 %d", n, got)
	}
}

// TestConcurrent_SameKeyPut 并发写同一 key：文件不损坏、终值为其中之一。
func TestConcurrent_SameKeyPut(t *testing.T) {
	s := newTestStore(t)
	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = s.Put("same", i, time.Minute)
		}(i)
	}
	wg.Wait()
	got := s.GetInt("same")
	if got < 0 || got >= n {
		t.Fatalf("终值应为 0..%d 之一，实际 %d", n-1, got)
	}
}

// TestConcurrent_FlushAndPut 并发 Flush 与写入不 panic。
func TestConcurrent_FlushAndPut(t *testing.T) {
	s := newTestStore(t)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _ = s.Flush() }()
		go func() { defer wg.Done(); _ = s.Put("k", "v", time.Minute) }()
	}
	wg.Wait()
}

// ── 契约完整性 ───────────────────────────────────────────────────────

// TestImplementsCacheStore 编译期断言 FileStore 实现契约。
func TestImplementsCacheStore(t *testing.T) {
	var _ contracts.CacheStore = (*FileStore)(nil)
	var _ contracts.TaggedCache = (*fileTaggedCache)(nil)
	var _ contracts.CacheLock = (*fileLock)(nil)
}
