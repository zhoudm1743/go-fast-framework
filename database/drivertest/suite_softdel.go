// suite_softdel.go 软删除一致性矩阵（orm-tag-design.md §11.9 全行）。
// 两种形态：旧版业务级（deleted_at 列无 sd 标记，软删标记由业务经 Update
// 写入，查询显式 Where("deleted_at = ?", 0)）与框架托管（sd tag，文档 4.5，
// Delete 自动改写置位 UPDATE、默认查询自动过滤存活行，双驱动一致）。
// 框架提供 OnlyTrashed/Restore/ForceDelete/Unscoped 扩展（两种形态均适用）。

package drivertest

import (
	"testing"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// seedSoftDel 建 sd1(存活) sd2(软删) sd3(存活)；o1(存活)/o4(软删) 两订单挂 u1
// 供"软删 + Preload 子表过滤"。
func seedSoftDel(t *testing.T, f Factory) contracts.Driver {
	t.Helper()
	drv := f(t)
	mustAutoMigrate(t, drv, &SuiteSoftDel{}, &SuiteUser{}, &SuiteOrder{})
	q := drv.Query()
	mustCreate(t, q,
		&SuiteSoftDel{ID: "sd1", Name: "alive-1"},
		&SuiteSoftDel{ID: "sd2", Name: "trashed"},
		&SuiteSoftDel{ID: "sd3", Name: "alive-2"},
		&SuiteUser{ID: "u1", Name: "alice", Email: "alice@test.dev"},
		&SuiteOrder{ID: "o1", SuiteUserID: "u1", Amount: 100},
		&SuiteOrder{ID: "o4", SuiteUserID: "u1", Amount: 999},
	)
	// 业务级软删标记：deleted_at 置为当前秒（≠0 即视为已删）
	now := time.Now().Unix()
	mustExec(t, q, "UPDATE suite_soft_dels SET deleted_at = ? WHERE id = ?", now, "sd2")
	mustExec(t, q, "UPDATE suite_orders SET deleted_at = ? WHERE id = ?", now, "o4")
	return drv
}

func suiteSoftDelete(t *testing.T, f Factory) {
	t.Run("软删后默认查询按业务 scope 不可见", func(t *testing.T) {
		drv := seedSoftDel(t, f)
		q := drv.Query()
		// 框架软删除为业务级：默认查询配 deleted_at = 0 scope（§11.9 语义）
		var alive []SuiteSoftDel
		if err := q.Model(&SuiteSoftDel{}).Where("deleted_at = ?", 0).Order("id").Find(&alive); err != nil {
			t.Fatalf("存活查询: %v", err)
		}
		if len(alive) != 2 || alive[0].ID != "sd1" || alive[1].ID != "sd3" {
			t.Fatalf("软删行应不可见，期望 [sd1 sd3]，实际 %+v", softIDs(alive))
		}
		var count int64
		if err := q.Model(&SuiteSoftDel{}).Where("deleted_at = ?", 0).Count(&count); err != nil || count != 2 {
			t.Fatalf("存活 Count 期望 2，实际 %d (err=%v)", count, err)
		}
	})

	t.Run("Unscoped 可见全部", func(t *testing.T) {
		drv := seedSoftDel(t, f)
		var all []SuiteSoftDel
		if err := drv.Query().Model(&SuiteSoftDel{}).Unscoped().Order("id").Find(&all); err != nil {
			t.Fatalf("Unscoped Find: %v", err)
		}
		if len(all) != 3 {
			t.Fatalf("Unscoped 应含已删行（期望 3），实际 %d", len(all))
		}
	})

	t.Run("OnlyTrashed 仅已删行", func(t *testing.T) {
		drv := seedSoftDel(t, f)
		var trashed []SuiteSoftDel
		if err := drv.Query().Model(&SuiteSoftDel{}).OnlyTrashed().Find(&trashed); err != nil {
			t.Fatalf("OnlyTrashed: %v", err)
		}
		if len(trashed) != 1 || trashed[0].ID != "sd2" {
			t.Fatalf("OnlyTrashed 期望 [sd2]，实际 %+v", softIDs(trashed))
		}
	})

	t.Run("Restore 恢复后重新可见", func(t *testing.T) {
		drv := seedSoftDel(t, f)
		q := drv.Query()
		if err := q.Model(&SuiteSoftDel{}).Where("id = ?", "sd2").Restore(); err != nil {
			t.Fatalf("Restore（需显式 Table/Model）: %v", err)
		}
		var got SuiteSoftDel
		mustTake(t, q, &got, "sd2")
		if got.DeletedAt != 0 {
			t.Fatalf("Restore 后 deleted_at 应置 0，实际 %d", got.DeletedAt)
		}
		var count int64
		if err := q.Model(&SuiteSoftDel{}).Where("deleted_at = ?", 0).Count(&count); err != nil || count != 3 {
			t.Fatalf("恢复后存活 Count 期望 3，实际 %d (err=%v)", count, err)
		}
	})

	t.Run("ForceDelete 物理删除", func(t *testing.T) {
		drv := seedSoftDel(t, f)
		q := drv.Query()
		if err := q.ForceDelete(&SuiteSoftDel{}, "id = ?", "sd2"); err != nil {
			t.Fatalf("ForceDelete: %v", err)
		}
		var all []SuiteSoftDel
		if err := q.Model(&SuiteSoftDel{}).Unscoped().Find(&all); err != nil {
			t.Fatalf("Unscoped Find: %v", err)
		}
		if len(all) != 2 {
			t.Fatalf("ForceDelete 后 Unscoped 也应查不到（期望 2），实际 %d", len(all))
		}
	})

	t.Run("软删 + Preload 子表过滤", func(t *testing.T) {
		drv := seedSoftDel(t, f)
		var rows []SuiteUser
		// 子表过滤经 Preload conds 表达（双驱动一致：gorm 原生内联条件 / 共享
		// 引擎 conds 作用于子查询）
		err := drv.Query().Model(&SuiteUser{}).Where("id = ?", "u1").
			Preload("Orders", "deleted_at = ?", 0).Find(&rows)
		if err != nil {
			t.Fatalf("软删 + Preload: %v", err)
		}
		if len(rows) != 1 || len(rows[0].Orders) != 1 || rows[0].Orders[0].ID != "o1" {
			t.Fatalf("软删子行不应回填，期望 [o1]，实际 %+v", rows[0].Orders)
		}
	})

	t.Run("软删 + Preload 引擎路径自动过滤", func(t *testing.T) {
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteOrder{}, &SuitePhoto{})
		q := drv.Query()
		mustCreate(t, q, &SuiteOrder{ID: "so1", SuiteUserID: "u1", Amount: 1})
		mustCreate(t, q,
			&SuitePhoto{ID: "ph1", OwnerID: "so1", OwnerType: "suite_orders", URL: "alive.png"},
			&SuitePhoto{ID: "ph4", OwnerID: "so1", OwnerType: "suite_orders", URL: "trashed.png"},
		)
		now := time.Now().Unix()
		mustExec(t, q, "UPDATE suite_photos SET deleted_at = ? WHERE id = ?", now, "ph4")
		var orders []SuiteOrder
		// 不传 conds：SuiteOrder.Photos 带 gorm:"-"，双驱动均走共享引擎，
		// 业务级 deleted_at 子表须自动过滤（§11.9「软删除模型 + Preload」行）。
		if err := q.Model(&SuiteOrder{}).Where("id = ?", "so1").Preload("Photos").Find(&orders); err != nil {
			t.Fatalf("Preload(Photos): %v", err)
		}
		if len(orders) != 1 || len(orders[0].Photos) != 1 || orders[0].Photos[0].ID != "ph1" {
			t.Fatalf("软删子行不应回填，期望 [ph1]，实际 %+v", orders[0].Photos)
		}
	})

	// TODO(11.9)：软删 + 查询缓存失效——依赖查询缓存启用，见 suiteCache 组
	// "写操作失效（软删标记路径）"用例。

	// 框架托管软删（sd tag，五种模式矩阵 + 嵌套嵌入 + Preload 自动过滤）。
	suiteSoftDeleteManaged(t, f)
}

func softIDs(rows []SuiteSoftDel) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out
}

// ── 框架托管软删（sd tag，§4.5/§11.9）──────────────────────────────────
//
// 五种值模式矩阵：Delete 自动改写置位 UPDATE、默认查询自动过滤存活行、
// Unscoped 绕过、OnlyTrashed/Restore 类型感知、Update 跳过已删行、
// ForceDelete 物理删除；另覆盖嵌套嵌入模型的 sd 解析与 sd 子表 Preload
// 自动过滤（time 模式 IS NULL 条件路径）。

func suiteSoftDeleteManaged(t *testing.T, f Factory) {
	t.Run("sd 秒级：Delete 改写/过滤/恢复", func(t *testing.T) {
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteSdSec{})
		q := drv.Query()
		mustCreate(t, q, &SuiteSdSec{ID: "s1", Name: "alive"}, &SuiteSdSec{ID: "s2", Name: "trash"})

		if err := q.Model(&SuiteSdSec{}).Delete(&SuiteSdSec{ID: "s2"}); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		var alive []SuiteSdSec
		if err := q.Model(&SuiteSdSec{}).Order("id").Find(&alive); err != nil {
			t.Fatalf("存活查询: %v", err)
		}
		if len(alive) != 1 || alive[0].ID != "s1" {
			t.Fatalf("默认查询应只含 s1，实际 %+v", alive)
		}
		var count int64
		if err := q.Model(&SuiteSdSec{}).Count(&count); err != nil || count != 1 {
			t.Fatalf("存活 Count 期望 1，实际 %d (err=%v)", count, err)
		}
		var all []SuiteSdSec
		if err := q.Model(&SuiteSdSec{}).Unscoped().Order("id").Find(&all); err != nil {
			t.Fatalf("Unscoped: %v", err)
		}
		if len(all) != 2 {
			t.Fatalf("Unscoped 应含 2 行，实际 %d", len(all))
		}
		now := time.Now().Unix()
		if all[1].DeletedAt == 0 || all[1].DeletedAt > now {
			t.Fatalf("软删值应为 <= 当前 Unix 秒，实际 %d（now=%d）", all[1].DeletedAt, now)
		}
		var trashed []SuiteSdSec
		if err := q.Model(&SuiteSdSec{}).OnlyTrashed().Find(&trashed); err != nil {
			t.Fatalf("OnlyTrashed: %v", err)
		}
		if len(trashed) != 1 || trashed[0].ID != "s2" {
			t.Fatalf("OnlyTrashed 应只含 s2，实际 %+v", trashed)
		}
		if err := q.Model(&SuiteSdSec{}).Where("id = ?", "s2").Restore(); err != nil {
			t.Fatalf("Restore: %v", err)
		}
		var restored []SuiteSdSec
		if err := q.Model(&SuiteSdSec{}).Order("id").Find(&restored); err != nil || len(restored) != 2 {
			t.Fatalf("Restore 后默认查询应含 2 行，实际 %+v (err=%v)", restored, err)
		}
	})

	t.Run("sd 毫秒/纳秒/标记：置位值模式正确", func(t *testing.T) {
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteSdMilli{}, &SuiteSdNano{}, &SuiteSdFlag{})
		q := drv.Query()
		mustCreate(t, q, &SuiteSdMilli{ID: "m1"}, &SuiteSdNano{ID: "n1"}, &SuiteSdFlag{ID: "f1"})

		if err := q.Model(&SuiteSdMilli{}).Delete(&SuiteSdMilli{ID: "m1"}); err != nil {
			t.Fatalf("Delete milli: %v", err)
		}
		var m SuiteSdMilli
		mustTake(t, q.Model(&SuiteSdMilli{}).Unscoped(), &m, "m1")
		if m.DeletedAt == 0 || m.DeletedAt > time.Now().UnixMilli() {
			t.Fatalf("milli 软删值应为 <= 当前 Unix 毫秒，实际 %d", m.DeletedAt)
		}

		if err := q.Model(&SuiteSdNano{}).Delete(&SuiteSdNano{ID: "n1"}); err != nil {
			t.Fatalf("Delete nano: %v", err)
		}
		var n SuiteSdNano
		mustTake(t, q.Model(&SuiteSdNano{}).Unscoped(), &n, "n1")
		if n.DeletedAt == 0 || n.DeletedAt > time.Now().UnixNano() {
			t.Fatalf("nano 软删值应为 <= 当前 Unix 纳秒，实际 %d", n.DeletedAt)
		}

		if err := q.Model(&SuiteSdFlag{}).Delete(&SuiteSdFlag{ID: "f1"}); err != nil {
			t.Fatalf("Delete flag: %v", err)
		}
		var fl SuiteSdFlag
		mustTake(t, q.Model(&SuiteSdFlag{}).Unscoped(), &fl, "f1")
		if fl.DeletedAt != 1 {
			t.Fatalf("flag 软删值应为 1，实际 %d", fl.DeletedAt)
		}
		var alive []SuiteSdFlag
		if err := q.Model(&SuiteSdFlag{}).Find(&alive); err != nil || len(alive) != 0 {
			t.Fatalf("flag 默认查询应为空，实际 %+v (err=%v)", alive, err)
		}
	})

	t.Run("sd time：NULL 语义全链路", func(t *testing.T) {
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteSdTime{})
		q := drv.Query()
		mustCreate(t, q, &SuiteSdTime{ID: "t1", Name: "alive"}, &SuiteSdTime{ID: "t2", Name: "trash"})

		if err := q.Model(&SuiteSdTime{}).Delete(&SuiteSdTime{ID: "t2"}); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		var alive []SuiteSdTime
		if err := q.Model(&SuiteSdTime{}).Find(&alive); err != nil {
			t.Fatalf("存活查询: %v", err)
		}
		if len(alive) != 1 || alive[0].ID != "t1" || alive[0].DeletedAt != nil {
			t.Fatalf("默认查询应只含未删 t1 且 DeletedAt 为 NULL，实际 %+v", alive)
		}
		var trashed []SuiteSdTime
		if err := q.Model(&SuiteSdTime{}).OnlyTrashed().Find(&trashed); err != nil {
			t.Fatalf("OnlyTrashed: %v", err)
		}
		if len(trashed) != 1 || trashed[0].ID != "t2" || trashed[0].DeletedAt == nil {
			t.Fatalf("OnlyTrashed 应只含已删 t2 且时间非 NULL，实际 %+v", trashed)
		}
		if err := q.Model(&SuiteSdTime{}).Where("id = ?", "t2").Restore(); err != nil {
			t.Fatalf("Restore: %v", err)
		}
		var t2 SuiteSdTime
		mustTake(t, q, &t2, "t2")
		if t2.DeletedAt != nil {
			t.Fatalf("Restore 后 DeletedAt 应为 NULL，实际 %v", t2.DeletedAt)
		}
	})

	t.Run("sd Update 跳过已删行", func(t *testing.T) {
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteSdSec{})
		q := drv.Query()
		mustCreate(t, q, &SuiteSdSec{ID: "u1", Name: "orig"})
		if err := q.Model(&SuiteSdSec{}).Delete(&SuiteSdSec{ID: "u1"}); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if err := q.Model(&SuiteSdSec{}).Where("id = ?", "u1").Update("name", "hacked"); err != nil {
			t.Fatalf("Update: %v", err)
		}
		var r SuiteSdSec
		mustTake(t, q.Model(&SuiteSdSec{}).Unscoped(), &r, "u1")
		if r.Name != "orig" {
			t.Fatalf("Update 不应触及已删行，实际 name=%q", r.Name)
		}
	})

	t.Run("sd ForceDelete 物理删除", func(t *testing.T) {
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteSdSec{})
		q := drv.Query()
		mustCreate(t, q, &SuiteSdSec{ID: "d1"}, &SuiteSdSec{ID: "d2"})
		if err := q.Model(&SuiteSdSec{}).Delete(&SuiteSdSec{ID: "d1"}); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if err := q.Model(&SuiteSdSec{}).ForceDelete(&SuiteSdSec{}, "id = ?", "d1"); err != nil {
			t.Fatalf("ForceDelete: %v", err)
		}
		var all []SuiteSdSec
		if err := q.Model(&SuiteSdSec{}).Unscoped().Find(&all); err != nil {
			t.Fatalf("Unscoped: %v", err)
		}
		if len(all) != 1 || all[0].ID != "d2" {
			t.Fatalf("ForceDelete 应物理删除 d1，实际 %+v", all)
		}
	})

	t.Run("sd 嵌套嵌入模型", func(t *testing.T) {
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteSdNest{})
		q := drv.Query()
		mustCreate(t, q,
			&SuiteSdNest{SuiteSdNestMid: SuiteSdNestMid{SuiteSdNestInner: SuiteSdNestInner{Name: "keep"}}},
			&SuiteSdNest{SuiteSdNestMid: SuiteSdNestMid{SuiteSdNestInner: SuiteSdNestInner{Name: "drop"}}},
		)
		if err := q.Model(&SuiteSdNest{}).Delete(&SuiteSdNest{
			SuiteSdNestMid: SuiteSdNestMid{ID: lastNestID(t, q, "drop")},
		}); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		var alive []SuiteSdNest
		if err := q.Model(&SuiteSdNest{}).Find(&alive); err != nil {
			t.Fatalf("存活查询: %v", err)
		}
		if len(alive) != 1 || alive[0].Name != "keep" {
			t.Fatalf("嵌套嵌入 sd 默认查询应只含 keep，实际 %+v", alive)
		}
	})

	t.Run("sd 子表 Preload 自动过滤（time 模式）", func(t *testing.T) {
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteSdOrder{}, &SuiteSdPhoto{})
		q := drv.Query()
		mustCreate(t, q,
			&SuiteSdOrder{ID: "po1", Name: "order"},
			&SuiteSdPhoto{ID: "pp1", OrderID: "po1", URL: "alive.png"},
			&SuiteSdPhoto{ID: "pp2", OrderID: "po1", URL: "trashed.png"},
		)
		if err := q.Model(&SuiteSdPhoto{}).Delete(&SuiteSdPhoto{ID: "pp2"}); err != nil {
			t.Fatalf("Delete 子表: %v", err)
		}
		var orders []SuiteSdOrder
		if err := q.Model(&SuiteSdOrder{}).Where("id = ?", "po1").Preload("Photos").Find(&orders); err != nil {
			t.Fatalf("Preload(Photos): %v", err)
		}
		if len(orders) != 1 || len(orders[0].Photos) != 1 || orders[0].Photos[0].ID != "pp1" {
			t.Fatalf("sd 子表软删行不应回填，期望 [pp1]，实际 %+v", orders[0].Photos)
		}
	})
}

// lastNestID 取嵌套模型中指定 Name 的行 ID（Delete 用 bean 定位）。
func lastNestID(t *testing.T, q contracts.Query, name string) string {
	t.Helper()
	var rows []SuiteSdNest
	if err := q.Model(&SuiteSdNest{}).Where("name = ?", name).Find(&rows); err != nil {
		t.Fatalf("查询嵌套模型: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("期望 1 行 name=%s，实际 %d", name, len(rows))
	}
	return rows[0].ID
}
