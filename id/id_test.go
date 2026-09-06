package id_test

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zhoudm1743/go-fast-framework/id"
)

// ── 基本属性 ──────────────────────────────────────────────────────────

func TestNew_Length(t *testing.T) {
	for i := 0; i < 200; i++ {
		if got := id.New(); len(got) != id.Size {
			t.Fatalf("expected length %d, got %d: %q", id.Size, len(got), got)
		}
	}
}

func TestNew_ValidCharset(t *testing.T) {
	const valid = "0123456789abcdefghjkmnpqrstvwxyz"
	lookup := make(map[rune]struct{}, 32)
	for _, c := range valid {
		lookup[c] = struct{}{}
	}
	for i := 0; i < 2000; i++ {
		for pos, c := range id.New() {
			if _, ok := lookup[c]; !ok {
				t.Fatalf("invalid character %q at position %d", c, pos)
			}
		}
	}
}

func TestNew_Unique(t *testing.T) {
	const n = 100_000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		v := id.New()
		if _, dup := seen[v]; dup {
			t.Fatalf("duplicate id at i=%d: %q", i, v)
		}
		seen[v] = struct{}{}
	}
}

func TestNew_NeverEmpty(t *testing.T) {
	for i := 0; i < 100; i++ {
		if v := id.New(); v == "" {
			t.Fatal("got empty id")
		}
	}
}

// ── 有序性 ────────────────────────────────────────────────────────────

// TestNew_StrictlyMonotonic 连续生成 10 万个 ID，必须严格单调递增（无重复）。
func TestNew_StrictlyMonotonic(t *testing.T) {
	const n = 100_000
	ids := make([]string, n)
	for i := range ids {
		ids[i] = id.New()
	}
	for i := 1; i < n; i++ {
		if ids[i] <= ids[i-1] {
			t.Fatalf("not strictly monotonic at %d:\n  prev=%q\n  curr=%q", i, ids[i-1], ids[i])
		}
	}
}

// TestNew_CrossMs_Ordered 不同毫秒生成的 ID 必须后者字典序严格大于前者。
func TestNew_CrossMs_Ordered(t *testing.T) {
	a := id.New()
	time.Sleep(5 * time.Millisecond)
	b := id.New()
	if b <= a {
		t.Fatalf("cross-ms order violated: %q >= %q", b, a)
	}
}

// TestNew_SortConsistency sort.Strings 结果必须与生成顺序一致。
func TestNew_SortConsistency(t *testing.T) {
	const n = 2000
	ids := make([]string, n)
	for i := range ids {
		ids[i] = id.New()
	}
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	for i := range ids {
		if ids[i] != sorted[i] {
			t.Fatalf("sort mismatch at %d: gen=%q sorted=%q", i, ids[i], sorted[i])
		}
	}
}

// ── 时钟回拨保护 ───────────────────────────────────────────────────────

// TestNew_ClockBackward_Monotonic 模拟 NTP 时间回拨，验证 ID 仍严格单调递增。
func TestNew_ClockBackward_Monotonic(t *testing.T) {
	var tick atomic.Int64
	tick.Store(2_000_000_000) // 起始 ms（任意正值）

	id.SetNowFn(func() int64 { return tick.Load() })
	defer id.ResetNowFn()

	ids := make([]string, 0, 300)

	// 正常推进 100ms
	for i := 0; i < 100; i++ {
		tick.Add(1)
		ids = append(ids, id.New())
	}
	// 回拨 50ms
	tick.Add(-50)
	for i := 0; i < 100; i++ {
		ids = append(ids, id.New())
	}
	// 恢复并继续推进
	tick.Add(200)
	for i := 0; i < 100; i++ {
		tick.Add(1)
		ids = append(ids, id.New())
	}

	for i := 1; i < len(ids); i++ {
		if ids[i] <= ids[i-1] {
			t.Fatalf("clock backward broke monotonicity at %d:\n  prev=%q\n  curr=%q", i, ids[i-1], ids[i])
		}
	}
}

// TestNew_SameMsMonotonic 固定时间戳，验证同 ms 内序列严格递增。
func TestNew_SameMsMonotonic(t *testing.T) {
	id.SetNowFn(func() int64 { return 999_999_999 })
	defer id.ResetNowFn()

	const n = 50_000
	ids := make([]string, n)
	for i := range ids {
		ids[i] = id.New()
	}
	for i := 1; i < n; i++ {
		if ids[i] <= ids[i-1] {
			t.Fatalf("same-ms not monotonic at %d: %q <= %q", i, ids[i], ids[i-1])
		}
	}
}

// TestNew_SequenceOverflow_Handled 序列溢出时应推进虚拟时钟，不破坏单调性。
func TestNew_SequenceOverflow_Handled(t *testing.T) {
	// 固定 ms，生成足够多的 ID 以触发序列进入大值区间；
	// 然后通过 SetNowFn 使 lastMs 不再前进，消耗序列空间（有限验证）。
	var ts atomic.Int64
	ts.Store(5_555_555)
	id.SetNowFn(func() int64 { return ts.Load() })
	defer id.ResetNowFn()

	const n = 10_000
	ids := make([]string, n)
	for i := range ids {
		ids[i] = id.New()
	}
	for i := 1; i < n; i++ {
		if ids[i] <= ids[i-1] {
			t.Fatalf("overflow region not monotonic at %d: %q <= %q", i, ids[i], ids[i-1])
		}
	}
	// 验证 lastMs 只增不减（溢出时应推进了虚拟时钟）
	if lm := id.LastMs(); lm < 5_555_555 {
		t.Fatalf("lastMs should be >= initial ts, got %d", lm)
	}
}

// ── Parse ─────────────────────────────────────────────────────────────

func TestParse_RoundTrip(t *testing.T) {
	const n = 200
	for i := 0; i < n; i++ {
		before := time.Now().UnixMilli()
		v := id.New()
		after := time.Now().UnixMilli()

		ts, err := id.Parse(v)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		ms := ts.UnixMilli()
		if ms < before || ms > after {
			t.Fatalf("parsed ts %d out of range [%d, %d] for id=%q", ms, before, after, v)
		}
	}
}

func TestParse_InvalidLength(t *testing.T) {
	cases := []string{"", "short", "toolongforthistype000"}
	for _, bad := range cases {
		if _, err := id.Parse(bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}

func TestParse_InvalidCharacter(t *testing.T) {
	// 'u' 被排除在 charset 之外，且无纠错映射，应报错
	// ('i'/'l'/'o' 会被 Crockford 纠错映射为 1/1/0，属于合法输入)
	for _, c := range []byte("uU") {
		bad := fmt.Sprintf("00000000%c0000000", c)
		if _, err := id.Parse(bad); err == nil {
			t.Fatalf("expected error for id containing %q: %q", c, bad)
		}
	}
	// 其他不在 charset 中且无纠错映射的字符应报错
	for _, c := range []byte("_/.") {
		bad := fmt.Sprintf("00000000%c0000000", c)
		if _, err := id.Parse(bad); err == nil {
			t.Fatalf("expected error for id containing %q: %q", c, bad)
		}
	}
}

// ── Crockford 归一化测试 ──────────────────────────────────────────────

func TestParse_Uppercase(t *testing.T) {
	// 全大写 ID 应正确解析（Crockford 大小写不敏感）
	upper := "01JDM4QR0S2FGK01"
	ts, err := id.Parse(upper)
	if err != nil {
		t.Fatalf("大写 ID 解析失败: %v", err)
	}
	if ts.IsZero() {
		t.Fatal("解析结果不应为零值")
	}
}

func TestParse_MixedCase(t *testing.T) {
	// 大小写混合解析
	mixed := "01jDm4Qr0S2fGk01"
	ts, err := id.Parse(mixed)
	if err != nil {
		t.Fatalf("大小写混合 ID 解析失败: %v", err)
	}
	if ts.IsZero() {
		t.Fatal("解析结果不应为零值")
	}
}

func TestParse_CorrectionChar_I(t *testing.T) {
	// I/i 应映射为 1
	for _, c := range []string{"I", "i"} {
		// 构造 ID，将第 5 个字符替换为 I/i，其他用已知合法字符
		raw := id.New()
		modified := raw[:5] + c + raw[6:]
		ts, err := id.Parse(modified)
		if err != nil {
			t.Fatalf("纠错字符 %q 解析失败: %v", c, err)
		}
		_ = ts
	}
}

func TestParse_CorrectionChar_L(t *testing.T) {
	// L/l 应映射为 1
	for _, c := range []string{"L", "l"} {
		raw := id.New()
		modified := raw[:8] + c + raw[9:]
		_, err := id.Parse(modified)
		if err != nil {
			t.Fatalf("纠错字符 %q 解析失败: %v", c, err)
		}
	}
}

func TestParse_CorrectionChar_O(t *testing.T) {
	// O/o 应映射为 0
	for _, c := range []string{"O", "o"} {
		raw := id.New()
		modified := raw[:3] + c + raw[4:]
		_, err := id.Parse(modified)
		if err != nil {
			t.Fatalf("纠错字符 %q 解析失败: %v", c, err)
		}
	}
}

func TestParse_WithHyphens(t *testing.T) {
	// 连字符应被忽略
	raw := id.New()
	// 在不同位置插入连字符
	withHyphens := raw[:4] + "-" + raw[4:8] + "-" + raw[8:12] + "-" + raw[12:]
	ts1, err := id.Parse(withHyphens)
	if err != nil {
		t.Fatalf("含连字符 ID 解析失败: %v", err)
	}
	ts2, err := id.Parse(raw)
	if err != nil {
		t.Fatalf("原始 ID 解析失败: %v", err)
	}
	if !ts1.Equal(ts2) {
		t.Fatalf("含连字符的解析结果应与原始一致: %v vs %v", ts1, ts2)
	}
}

func TestParse_HyphensInvalidAfterStrip(t *testing.T) {
	// 连字符去除后长度不对仍应报错
	_, err := id.Parse("abc-def")
	if err == nil {
		t.Fatal("去除连字符后长度不足应报错")
	}
}

func TestParse_RoundTrip_Normalization(t *testing.T) {
	// 生成 ID → Parse 往返（所有 New 输出都是小写无连字符，归一化不应改变）
	for i := 0; i < 50; i++ {
		v := id.New()
		ts, err := id.Parse(v)
		if err != nil {
			t.Fatalf("Parse 往返失败: %v", err)
		}
		if ts.IsZero() {
			t.Fatal("往返解析结果不应为零值")
		}
	}
}

// ── 字典序/时序排序（模拟 TEXT 主键 ORDER BY，不依赖 ORM）──────────────

// TestLexOrder_Asc 顺序生成 500 个 ID，验证字典序升序 = 生成顺序
// （与 SQLite TEXT 主键 ORDER BY id ASC 语义一致）。
func TestLexOrder_Asc(t *testing.T) {
	const n = 500
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		ids[i] = id.New()
	}
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	for i := range ids {
		if sorted[i] != ids[i] {
			t.Fatalf("lex ASC mismatch at pos %d: got %q want %q", i, sorted[i], ids[i])
		}
	}
}

// TestLexOrder_Desc 验证字典序降序为生成顺序的严格逆序。
func TestLexOrder_Desc(t *testing.T) {
	const n = 200
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		ids[i] = id.New()
	}
	sorted := append([]string(nil), ids...)
	sort.Sort(sort.Reverse(sort.StringSlice(sorted)))
	for i := range ids {
		if sorted[i] != ids[n-1-i] {
			t.Fatalf("lex DESC mismatch at pos %d: got %q want %q", i, sorted[i], ids[n-1-i])
		}
	}
}

// TestLexOrder_FirstLast 验证字典序最小/最大分别对应最早/最晚生成的 ID。
func TestLexOrder_FirstLast(t *testing.T) {
	const n = 100
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		ids[i] = id.New()
	}
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	if sorted[0] != ids[0] {
		t.Fatalf("first: got %q want %q", sorted[0], ids[0])
	}
	if sorted[n-1] != ids[n-1] {
		t.Fatalf("last: got %q want %q", sorted[n-1], ids[n-1])
	}
}

// TestLexOrder_Range 验证按 ID 边界做字典序区间过滤。
func TestLexOrder_Range(t *testing.T) {
	const total = 100
	ids := make([]string, total)
	for i := 0; i < total; i++ {
		ids[i] = id.New()
	}
	lo, hi := ids[20], ids[79]
	var got []string
	for _, v := range ids {
		if v >= lo && v <= hi {
			got = append(got, v)
		}
	}
	sort.Strings(got)
	if len(got) != 60 {
		t.Fatalf("range: expected 60, got %d", len(got))
	}
	if got[0] != ids[20] || got[59] != ids[79] {
		t.Fatalf("range boundary wrong: first=%q last=%q", got[0], got[59])
	}
}

// ── 多租户并发排序 ────────────────────────────────────────────────────

// TestMultiTenant_ConcurrentLexOrder 模拟 4 个租户并发生成 ID，
// 验证：每个租户局部字典序与生成顺序一致；跨租户 ID 全局唯一。
func TestMultiTenant_ConcurrentLexOrder(t *testing.T) {
	const tenants = 4
	const rowsPerTenant = 200

	allIDs := make([][]string, tenants)
	var wg sync.WaitGroup
	wg.Add(tenants)

	for tenant := 0; tenant < tenants; tenant++ {
		tenant := tenant
		go func() {
			defer wg.Done()
			ids := make([]string, rowsPerTenant)
			for i := 0; i < rowsPerTenant; i++ {
				ids[i] = id.New()
			}
			allIDs[tenant] = ids
		}()
	}
	wg.Wait()

	for tenant, insertedIDs := range allIDs {
		sorted := append([]string(nil), insertedIDs...)
		sort.Strings(sorted)
		for j := range insertedIDs {
			if sorted[j] != insertedIDs[j] {
				t.Errorf("tenant %d pos %d: lex order mismatch got %q want %q",
					tenant, j, sorted[j], insertedIDs[j])
			}
		}
	}

	seen := make(map[string]int, tenants*rowsPerTenant)
	for tenant, ids := range allIDs {
		for _, v := range ids {
			if prev, dup := seen[v]; dup {
				t.Errorf("collision: id=%q in tenant %d and %d", v, prev, tenant)
			}
			seen[v] = tenant
		}
	}
}

// TestMultiTenant_GlobalMergeOrder 将所有租户的 ID 合并后排序，
// 验证合并后的排序是一个合法的全局时序（每个租户的局部顺序被保留）。
func TestMultiTenant_GlobalMergeOrder(t *testing.T) {
	const tenants = 6
	const rowsPerTenant = 50

	allIDs := make([][]string, tenants)
	var wg sync.WaitGroup
	wg.Add(tenants)
	for tenant := 0; tenant < tenants; tenant++ {
		tenant := tenant
		go func() {
			defer wg.Done()
			ids := make([]string, rowsPerTenant)
			for i := range ids {
				ids[i] = id.New()
			}
			allIDs[tenant] = ids
		}()
	}
	wg.Wait()

	type taggedID struct {
		v      string
		tenant int
		seq    int
	}
	all := make([]taggedID, 0, tenants*rowsPerTenant)
	for tenant, ids := range allIDs {
		for seq, v := range ids {
			all = append(all, taggedID{v, tenant, seq})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].v < all[j].v })

	tenantLastSeq := make([]int, tenants)
	for i := range tenantLastSeq {
		tenantLastSeq[i] = -1
	}
	for _, item := range all {
		if item.seq <= tenantLastSeq[item.tenant] {
			t.Errorf("tenant %d local order violated in global merge: seq %d after %d",
				item.tenant, item.seq, tenantLastSeq[item.tenant])
		}
		tenantLastSeq[item.tenant] = item.seq
	}
}

// ── 并发安全 ──────────────────────────────────────────────────────────

// TestConcurrent_UniqueAndNoRace 64 协程各生成 1000 个 ID，全局无重复。
func TestConcurrent_UniqueAndNoRace(t *testing.T) {
	const workers, perWorker = 64, 1000
	var wg sync.WaitGroup
	wg.Add(workers)
	results := make([][]string, workers)
	for i := 0; i < workers; i++ {
		i := i
		go func() {
			defer wg.Done()
			buf := make([]string, perWorker)
			for j := range buf {
				buf[j] = id.New()
			}
			results[i] = buf
		}()
	}
	wg.Wait()

	seen := make(map[string]struct{}, workers*perWorker)
	for wi, buf := range results {
		for _, v := range buf {
			if _, dup := seen[v]; dup {
				t.Fatalf("duplicate from worker %d: %q", wi, v)
			}
			seen[v] = struct{}{}
		}
	}
}

// TestConcurrent_Count 验证并发下每个 goroutine 都能成功返回（无死锁/panic）。
func TestConcurrent_Count(t *testing.T) {
	var count atomic.Int64
	var wg sync.WaitGroup
	const n = 2000
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_ = id.New()
			count.Add(1)
		}()
	}
	wg.Wait()
	if count.Load() != n {
		t.Fatalf("expected %d completions, got %d", n, count.Load())
	}
}

// ── 边界 ─────────────────────────────────────────────────────────────

// TestNew_ZeroTimestamp 当 nowFn 返回 0（Unix 纪元），ID 仍合法且可解析。
func TestNew_ZeroTimestamp(t *testing.T) {
	id.SetNowFn(func() int64 { return 0 })
	defer id.ResetNowFn()

	v := id.New()
	if len(v) != id.Size {
		t.Fatalf("bad length: %q", v)
	}
	ts, err := id.Parse(v)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ts.UnixMilli() != 0 {
		t.Fatalf("expected ts=0, got %d", ts.UnixMilli())
	}
}

// TestNew_FarFutureTimestamp 50-bit 最大值附近不溢出。
func TestNew_FarFutureTimestamp(t *testing.T) {
	maxTs := int64((uint64(1) << 50) - 1)
	id.SetNowFn(func() int64 { return maxTs })
	defer id.ResetNowFn()

	v := id.New()
	if len(v) != id.Size {
		t.Fatalf("bad length: %q", v)
	}
	ts, err := id.Parse(v)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ts.UnixMilli() != maxTs {
		t.Fatalf("timestamp mismatch: got %d want %d", ts.UnixMilli(), maxTs)
	}
}

// ── 基准 ─────────────────────────────────────────────────────────────

func BenchmarkNew_Serial(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = id.New()
	}
}

func BenchmarkNew_Parallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = id.New()
		}
	})
}
