// suite_aggregate.go 聚合一致性组（orm-tag-design.md §11.3 聚合行 / 双驱动测试
// 方案 §六 第 6 项 AGG-01~05 的 SQLite 子集）。只断言双驱动共同语义子集：
//   - AGG-01：Group + Count 分组数、逐组计数（Scan 投影）；
//   - AGG-02：Having 内联常量与列条件——不带参。带参占位符为差异固化项
//     （gorm 完整支持 / xorm Session.Having 仅收 string 链上拒绝，见两驱动
//     fullcov_chain 差异断言），套件不纳入；
//   - AGG-03：count(*)/sum/avg/max/min 经 Scan 投影结构体与 ScanMap 回收
//     （ScanMap 数值经 suiteMapNum 归一后宽松断言，抹平驱动数值形态差异）；
//   - AGG-04：聚合 + Where 前置过滤、Order+Limit TopN 组；
//   - AGG-05：空表聚合形态（无 Group 单行 count=0 / sum 为 NULL 经双驱动
//     归一为 0；带 Group 0 行）。

package drivertest

import (
	"strconv"
	"strings"
	"testing"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// suiteMapStr ScanMap 文本值归一：string / []byte → string，其余返回空串
// （与驱动侧 fullcov 的 fc5MapStr 同口径）。
func suiteMapStr(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	}
	return ""
}

// suiteMapNum ScanMap 数值归一：int 系 / float 系 / []byte / 数字串 → int64。
// SQLite 各驱动对聚合结果的扫描形态不一（int64/float64/文本），断言一律经本
// helper 折算后比较。
func suiteMapNum(t *testing.T, label string, v any) int64 {
	t.Helper()
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case int32:
		return int64(n)
	case uint64:
		return int64(n)
	case float64:
		return int64(n)
	case []byte:
		return suiteParseNum(t, label, string(n))
	case string:
		return suiteParseNum(t, label, n)
	case nil:
		return 0
	}
	t.Fatalf("%s: 无法归一的数值形态 %#v", label, v)
	return 0
}

// suiteParseNum 数字串解析（容忍两端空白），失败即 Fatal。
func suiteParseNum(t *testing.T, label, s string) int64 {
	t.Helper()
	s = strings.TrimSpace(s)
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int64(f)
	}
	t.Fatalf("%s: 数字串 %q 解析失败", label, s)
	return 0
}

// seedSuiteAggs 建 SuiteAgg 基准数据（6 行）：
//
//	a×3（amount 10/20/30，组和 60）、b×2（5/15，组和 20）、c×1（100）。
//	全表聚合基准：count=6、sum=180、avg=30、max=100、min=5。
func seedSuiteAggs(t *testing.T, f Factory) contracts.Driver {
	t.Helper()
	drv := f(t)
	mustAutoMigrate(t, drv, &SuiteAgg{})
	q := drv.Query()
	mustCreate(t, q,
		&SuiteAgg{ID: "agg-a1", Category: "a", Amount: 10},
		&SuiteAgg{ID: "agg-a2", Category: "a", Amount: 20},
		&SuiteAgg{ID: "agg-a3", Category: "a", Amount: 30},
		&SuiteAgg{ID: "agg-b1", Category: "b", Amount: 5},
		&SuiteAgg{ID: "agg-b2", Category: "b", Amount: 15},
		&SuiteAgg{ID: "agg-c1", Category: "c", Amount: 100},
	)
	return drv
}

func suiteAggregate(t *testing.T, f Factory) {
	t.Run("Group+Count 分组统计", func(t *testing.T) {
		drv := seedSuiteAggs(t, f)
		q := drv.Query()

		// AGG-01：Group 后 Count 返回分组数（3 组），而非总行数 6
		var n int64
		if err := q.Model(&SuiteAgg{}).Group("category").Count(&n); err != nil {
			t.Fatalf("Group+Count: %v", err)
		}
		if n != 3 {
			t.Fatalf("Group 后 Count 期望 3 组，实际 %d", n)
		}

		// 逐组计数回读：分组正确性（a:3、b:2、c:1），按组名排序稳定断言
		var aggs []struct {
			Category string `orm:"'category'"`
			Cnt      int64  `orm:"'cnt'"`
		}
		if err := q.Model(&SuiteAgg{}).
			Select("category, count(*) AS cnt").
			Group("category").Order("category").
			Scan(&aggs); err != nil {
			t.Fatalf("Group+Scan: %v", err)
		}
		if len(aggs) != 3 {
			t.Fatalf("分组聚合期望 3 组，实际 %+v", aggs)
		}
		want := []struct {
			cat string
			cnt int64
		}{{"a", 3}, {"b", 2}, {"c", 1}}
		for i, w := range want {
			if aggs[i].Category != w.cat || aggs[i].Cnt != w.cnt {
				t.Fatalf("第 %d 组期望 {%s %d}，实际 %+v", i, w.cat, w.cnt, aggs[i])
			}
		}
	})

	t.Run("Having 内联条件过滤（无参共同子集）", func(t *testing.T) {
		drv := seedSuiteAggs(t, f)
		q := drv.Query()

		// AGG-02：内联常量条件——仅 count > 1 的组 {a, b} 剩下
		var aggs []struct {
			Category string `orm:"'category'"`
			Cnt      int64  `orm:"'cnt'"`
		}
		if err := q.Model(&SuiteAgg{}).
			Select("category, count(*) AS cnt").
			Group("category").
			Having("count(*) > 1").
			Order("category").
			Scan(&aggs); err != nil {
			t.Fatalf("Having(聚合条件): %v", err)
		}
		if len(aggs) != 2 || aggs[0].Category != "a" || aggs[1].Category != "b" {
			t.Fatalf("Having count(*) > 1 期望 [a b]，实际 %+v", aggs)
		}
		for _, a := range aggs {
			if a.Cnt <= 1 {
				t.Fatalf("Having 过滤后不应剩 cnt<=1 的组: %+v", aggs)
			}
		}

		// 列条件（内联常量）：剔除 c 组
		aggs = nil
		if err := q.Model(&SuiteAgg{}).
			Select("category, count(*) AS cnt").
			Group("category").
			Having("category <> 'c'").
			Order("category").
			Scan(&aggs); err != nil {
			t.Fatalf("Having(列条件): %v", err)
		}
		if len(aggs) != 2 || aggs[0].Category != "a" || aggs[1].Category != "b" {
			t.Fatalf("Having category <> 'c' 期望 [a b]，实际 %+v", aggs)
		}
		// 差异固化（不纳入套件）：Having 带参占位符——gorm 支持 / xorm 链上
		// ErrUnsupported，见两驱动 fullcov_chain 差异断言（方案 §六 AGG-02）。
	})

	t.Run("count/sum/avg/max/min 经 Scan 回收", func(t *testing.T) {
		drv := seedSuiteAggs(t, f)
		q := drv.Query()

		// AGG-03：标量聚合投影单行（基准 6/180/30/100/5）
		var agg struct {
			Cnt       int64   `orm:"'cnt'"`
			Total     int64   `orm:"'total'"`
			AvgAmount float64 `orm:"'avg_amount'"`
			MaxAmount int64   `orm:"'max_amount'"`
			MinAmount int64   `orm:"'min_amount'"`
		}
		if err := q.Model(&SuiteAgg{}).
			Select("count(*) AS cnt, sum(amount) AS total, avg(amount) AS avg_amount, " +
				"max(amount) AS max_amount, min(amount) AS min_amount").
			Scan(&agg); err != nil {
			t.Fatalf("Scan 聚合: %v", err)
		}
		if agg.Cnt != 6 || agg.Total != 180 || agg.MaxAmount != 100 || agg.MinAmount != 5 {
			t.Fatalf("聚合期望 {6 180 _ 100 5}，实际 %+v", agg)
		}
		if agg.AvgAmount != 30 {
			t.Fatalf("avg 期望 30，实际 %v", agg.AvgAmount)
		}
	})

	t.Run("聚合 ScanMap 数值归一回收", func(t *testing.T) {
		drv := seedSuiteAggs(t, f)
		q := drv.Query()

		// AGG-03：ScanMap 形态回收分组聚合（count/sum 数值列的驱动形态不一，
		// 经 suiteMapNum 归一后比较）
		var maps []map[string]any
		if err := q.Model(&SuiteAgg{}).
			Select("category, count(*) AS cnt, sum(amount) AS total").
			Group("category").Order("category").
			ScanMap(&maps); err != nil {
			t.Fatalf("ScanMap 聚合: %v", err)
		}
		if len(maps) != 3 {
			t.Fatalf("ScanMap 聚合期望 3 组，实际 %d: %v", len(maps), maps)
		}
		wantCnt := map[string]int64{"a": 3, "b": 2, "c": 1}
		wantTotal := map[string]int64{"a": 60, "b": 20, "c": 100}
		for _, m := range maps {
			cat := suiteMapStr(m["category"])
			gotCnt := suiteMapNum(t, "cnt", m["cnt"])
			gotTotal := suiteMapNum(t, "total", m["total"])
			if wantCnt[cat] != gotCnt || wantTotal[cat] != gotTotal {
				t.Fatalf("分组 %q 期望 {cnt:%d total:%d}，实际 {cnt:%d total:%d}",
					cat, wantCnt[cat], wantTotal[cat], gotCnt, gotTotal)
			}
		}
	})

	t.Run("Where 前置过滤与 TopN 组", func(t *testing.T) {
		drv := seedSuiteAggs(t, f)
		q := drv.Query()

		// AGG-04：聚合前过滤——amount >= 20 剔除 a 组 10、b 组全量 → {a, c} 2 组
		var n int64
		if err := q.Model(&SuiteAgg{}).Where("amount >= ?", 20).
			Group("category").Count(&n); err != nil {
			t.Fatalf("Where+Group Count: %v", err)
		}
		if n != 2 {
			t.Fatalf("Where+Group Count 期望 2 组，实际 %d", n)
		}

		var aggs []struct {
			Category string `orm:"'category'"`
			Cnt      int64  `orm:"'cnt'"`
		}
		if err := q.Model(&SuiteAgg{}).
			Select("category, count(*) AS cnt").
			Where("amount >= ?", 20).
			Group("category").Order("category").
			Scan(&aggs); err != nil {
			t.Fatalf("Where+Group Scan: %v", err)
		}
		if len(aggs) != 2 || aggs[0].Category != "a" || aggs[0].Cnt != 2 || aggs[1].Category != "c" {
			t.Fatalf("Where+Group 期望 [{a 2} {c 1}]，实际 %+v", aggs)
		}

		// 组内 TopN：cnt 降序取第 1 组 → 全量基准下 a:3
		var top []struct {
			Category string `orm:"'category'"`
			Cnt      int64  `orm:"'cnt'"`
		}
		if err := q.Model(&SuiteAgg{}).
			Select("category, count(*) AS cnt").
			Group("category").
			Order("cnt DESC").Limit(1).
			Scan(&top); err != nil {
			t.Fatalf("TopN 组: %v", err)
		}
		if len(top) != 1 || top[0].Category != "a" || top[0].Cnt != 3 {
			t.Fatalf("TopN 组期望 [{a 3}]，实际 %+v", top)
		}
	})

	t.Run("空表聚合形态", func(t *testing.T) {
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteAgg{})
		q := drv.Query()

		// AGG-05：空表无 Group——聚合恒返回 1 行：count=0；sum 为 NULL，
		// 双驱动扫描均归一为零值（实测一致：gorm 双指针扫描 NULL→0，
		// xorm NULL→字段零值），一并断言
		var agg struct {
			Cnt   int64 `orm:"'cnt'"`
			Total int64 `orm:"'total'"`
		}
		if err := q.Model(&SuiteAgg{}).
			Select("count(*) AS cnt, sum(amount) AS total").
			Scan(&agg); err != nil {
			t.Fatalf("空表聚合: %v", err)
		}
		if agg.Cnt != 0 || agg.Total != 0 {
			t.Fatalf("空表聚合期望 {cnt:0 total:0}，实际 %+v", agg)
		}

		// 空表带 Group：0 行
		var eg []struct {
			Category string `orm:"'category'"`
			Cnt      int64  `orm:"'cnt'"`
		}
		if err := q.Model(&SuiteAgg{}).
			Select("category, count(*) AS cnt").
			Group("category").
			Scan(&eg); err != nil {
			t.Fatalf("空表分组聚合: %v", err)
		}
		if len(eg) != 0 {
			t.Fatalf("空表分组聚合应 0 行，实际 %+v", eg)
		}
	})
}
