// suite_softdel.go 软删除一致性矩阵（orm-tag-design.md §11.9 全行）。
// 框架软删除为业务级实现（deleted_at 列仅普通 index/default 列），软删标记由
// 业务经 Update 写入；框架提供 OnlyTrashed/Restore/ForceDelete/Unscoped 扩展。

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

	// TODO(11.9)：软删 + 查询缓存失效——依赖查询缓存启用，见 suiteCache 组
	// "写操作失效（软删标记路径）"用例。
}

func softIDs(rows []SuiteSoftDel) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out
}
