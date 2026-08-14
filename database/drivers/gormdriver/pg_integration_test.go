//go:build integration

package gormdriver

import (
	"os"
	"sync"
	"testing"

	"github.com/zhoudm1743/go-fast-framework/contracts"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// 本文件为 PostgreSQL 多租户集成测试，依赖真实数据库，默认不运行。
// 运行方式：设置环境变量 GOFAST_TEST_PG_DSN 后执行 `go test -tags integration`。
// 示例：
//
//	GOFAST_TEST_PG_DSN="postgres://user:pass@host:5432/db?sslmode=disable" \
//	  go test -tags integration ./database/drivers/gormdriver/

type AppUser struct {
	ID        string `gorm:"column:id;primaryKey"`
	CreatedAt int64  `gorm:"column:created_at"`
	UpdatedAt int64  `gorm:"column:updated_at"`
	DeletedAt int64  `gorm:"column:deleted_at"`
	Name      string `gorm:"column:name"`
	Email     string `gorm:"column:email"`
	Phone     string `gorm:"column:phone"`
}

type OrgNode struct {
	ID       string    `gorm:"column:id;primaryKey"`
	ParentID string    `gorm:"column:parent_id"`
	Name     string    `gorm:"column:name"`
	Children []OrgNode `gorm:"foreignKey:ParentID;references:ID"`
}

func newPG(t *testing.T) *GormDriver {
	t.Helper()
	dsn := os.Getenv("GOFAST_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("未设置 GOFAST_TEST_PG_DSN 环境变量，跳过 pgsql 集成测试")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{TablePrefix: "public."},
		PrepareStmt:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &GormDriver{db: db}
}
func TestPG_ReadOnlyMethods(t *testing.T) {
	drv := newPG(t)
	ten := "tenant_260563780"

	// Count
	var total int64
	if err := drv.Query().Schema(ten).Model(&AppUser{}).Count(&total); err != nil {
		t.Fatalf("Count: %v", err)
	}
	if total != 1 {
		t.Errorf("Count 期望 tenant=1, 实际 %d", total)
	}

	// Pluck
	var names []string
	if err := drv.Query().Schema(ten).Model(&AppUser{}).Pluck("name", &names); err != nil {
		t.Fatalf("Pluck: %v", err)
	}
	if len(names) != 1 {
		t.Errorf("Pluck 期望 1 条, 实际 %v", names)
	}

	// Scan 到 struct
	var scans []AppUser
	if err := drv.Query().Schema(ten).Model(&AppUser{}).Scan(&scans); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(scans) != 1 {
		t.Errorf("Scan 期望 1 条, 实际 %d", len(scans))
	}

	// ScanMap
	var maps []map[string]any
	if err := drv.Query().Schema(ten).Model(&AppUser{}).ScanMap(&maps); err != nil {
		t.Fatalf("ScanMap: %v", err)
	}
	if len(maps) != 1 {
		t.Errorf("ScanMap 期望 1 条, 实际 %d", len(maps))
	}

	// First/Last/Take/Find
	var one AppUser
	if err := drv.Query().Schema(ten).Model(&AppUser{}).First(&one); err != nil {
		t.Fatalf("First: %v", err)
	}
	if one.Name != "测试用户" {
		t.Errorf("First 应查到 tenant 数据, 实际 name=%q", one.Name)
	}

	// Exists
	ok, err := drv.Query().Schema(ten).Exists(&AppUser{})
	if err != nil || !ok {
		t.Errorf("Exists 应 true, ok=%v err=%v", ok, err)
	}
}

// DryRun 观察未 applySchema 的写操作表名
func TestPG_WriteSQLTableName(t *testing.T) {
	drv := newPG(t)
	ten := "tenant_260563780"

	// Update 表名
	q1 := drv.Query().Schema(ten).Model(&AppUser{}).Where("id = ?", "x")
	stmt := q1.(*GormQuery).db.Session(&gorm.Session{DryRun: true}).Update("name", "y").Statement
	t.Logf("Update SQL: %s", stmt.SQL.String())

	// Updates 表名
	q2 := drv.Query().Schema(ten).Model(&AppUser{}).Where("id = ?", "x")
	stmt2 := q2.(*GormQuery).db.Session(&gorm.Session{DryRun: true}).Updates(map[string]any{"name": "y"}).Statement
	t.Logf("Updates SQL: %s", stmt2.SQL.String())

	// Restore 表名
	q3 := drv.Query().Schema(ten).Model(&AppUser{}).Where("id = ?", "x")
	stmt3 := q3.(*GormQuery).db.Session(&gorm.Session{DryRun: true}).Update("deleted_at", 0).Statement
	t.Logf("Restore(Update) SQL: %s", stmt3.SQL.String())
}

// 并发多租户：不同租户 schema 是否串
func TestPG_ConcurrentTenants(t *testing.T) {
	drv := newPG(t)

	var wg sync.WaitGroup
	errs := make(chan string, 20)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var total int64
			if err := drv.Query().Schema("tenant_260563780").Model(&AppUser{}).Count(&total); err != nil {
				errs <- "tenant err: " + err.Error()
				return
			}
			if total != 1 {
				errs <- "tenant count 期望 1 实际 " + string(rune('0'+total))
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			var total int64
			// public 平台数据（不设租户 schema，走默认 public）
			if err := drv.Query().Model(&AppUser{}).Count(&total); err != nil {
				errs <- "public err: " + err.Error()
				return
			}
			if total != 3 {
				errs <- "public count 期望 3"
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}

func TestPG_PreloadTenant(t *testing.T) {
	drv := newPG(t)
	ten := "tenant_260563780"

	var roots []OrgNode
	err := drv.Query().Schema(ten).Model(&OrgNode{}).
		Where("parent_id IS NULL").Preload("Children").Find(&roots)
	if err != nil {
		t.Fatalf("Preload: %v", err)
	}
	if len(roots) != 1 {
		t.Fatalf("应 1 个根节点, 实际 %d", len(roots))
	}
	if roots[0].Name != "大房东" {
		t.Errorf("根节点应为大房东, 实际 %q", roots[0].Name)
	}
	t.Logf("根节点 %q 的 Children 数: %d", roots[0].Name, len(roots[0].Children))
	if len(roots[0].Children) == 0 {
		t.Error("Preload Children 应加载子节点（schema 穿透问题会导致为空）")
	}
	for _, c := range roots[0].Children {
		t.Logf("  child: %s (%s)", c.Name, c.ID)
	}
}

func TestPG_JoinsTenantSQL(t *testing.T) {
	drv := newPG(t)
	ten := "tenant_260563780"

	q := drv.Query().Schema(ten).Model(&OrgNode{}).Joins("Children").Where("org_nodes.name = ?", "大房东")
	stmt := q.(*GormQuery).db.Session(&gorm.Session{DryRun: true}).Find(&[]OrgNode{}).Statement
	t.Logf("Joins SQL: %s", stmt.SQL.String())
}

func TestPG_TransactionTenant(t *testing.T) {
	drv := newPG(t)
	ten := "tenant_260563780"

	err := drv.Query().Schema(ten).Transaction(func(tx contracts.Query) error {
		var total int64
		if err := tx.Model(&AppUser{}).Count(&total); err != nil {
			return err
		}
		if total != 1 {
			t.Errorf("事务内 tenant Count 期望 1, 实际 %d", total)
		}
		// 事务内 Find 也应正确
		var one AppUser
		if err := tx.Model(&AppUser{}).First(&one); err != nil {
			return err
		}
		if one.Name != "测试用户" {
			t.Errorf("事务内 First 应查 tenant, 实际 %q", one.Name)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("事务: %v", err)
	}
}

func TestPG_NestedPreload(t *testing.T) {
	drv := newPG(t)
	ten := "tenant_260563780"

	var roots []OrgNode
	err := drv.Query().Schema(ten).Model(&OrgNode{}).
		Where("parent_id IS NULL").Preload("Children.Children").Find(&roots)
	if err != nil {
		t.Fatalf("嵌套 Preload: %v", err)
	}
	if len(roots) != 1 {
		t.Fatalf("应 1 根节点, 实际 %d", len(roots))
	}
	t.Logf("根 %q children=%d", roots[0].Name, len(roots[0].Children))
	for _, c := range roots[0].Children {
		t.Logf("  %q 的 children=%d", c.Name, len(c.Children))
	}
}

func TestPG_ScanMapNullable(t *testing.T) {
	drv := newPG(t)
	ten := "tenant_260563780"

	var maps []map[string]any
	err := drv.Query().Schema(ten).Model(&AppUser{}).Where("id IS NOT NULL").ScanMap(&maps)
	if err != nil {
		t.Fatalf("ScanMap: %v", err)
	}
	if len(maps) == 0 {
		t.Fatal("应查到数据")
	}
	// 检查可空字段
	for _, m := range maps {
		t.Logf("avatar=%v nickname=%v wechat_open_id=%v", m["avatar"], m["nickname"], m["wechat_open_id"])
	}
}

func TestPG_FirstOrCreate_Existing(t *testing.T) {
	drv := newPG(t)
	ten := "tenant_260563780"

	// 用已存在的记录，FirstOrCreate 走 First 分支，不写库
	m := &AppUser{ID: "77f4bb72-9664-4ff4-a075-29204e67c848", Name: "不该被覆盖"}
	if err := drv.Query().Schema(ten).FirstOrCreate(m, "id = ?", m.ID); err != nil {
		t.Fatalf("FirstOrCreate: %v", err)
	}
	if m.Name != "测试用户" {
		t.Errorf("FirstOrCreate 应查到 tenant 已有记录, 实际 name=%q", m.Name)
	}
}
