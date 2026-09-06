// suite_crud.go 基础 CRUD 一致性矩阵（orm-tag-design.md §11.3 核心行）。
// 未覆盖行以注释标注 TODO(11.x)：见文档。

package drivertest

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// seedUsers 建 3 用户：u1(d1/alice/30/91.5)、u2(d1/bob/40/61.25)、u3(d2/carol/0/0)。
// u3 的 age/score 为零值（默认值/零值写入断言用）。
func seedUsers(t *testing.T, f Factory) contracts.Driver {
	t.Helper()
	drv := f(t)
	mustAutoMigrate(t, drv, &SuiteUser{}, &SuiteNamedCol{}, &SuiteDept{})
	q := drv.Query()
	mustExec(t, q, `INSERT INTO suite_depts (id, name) VALUES ('d1', '研发'), ('d2', '运维')`)
	mustCreate(t, q,
		&SuiteUser{ID: "u1", Name: "alice", Email: "alice@test.dev", Age: 30, Score: 91.5, DeptID: "d1", Password: "pw-alice"},
		&SuiteUser{ID: "u2", Name: "bob", Email: "bob@test.dev", Age: 40, Score: 61.25, DeptID: "d1", Password: "pw-bob"},
		&SuiteUser{ID: "u3", Name: "carol", Email: "carol@test.dev", DeptID: "d2"},
	)
	mustCreate(t, q,
		&SuiteNamedCol{ID: "n1", Name: "张三", Amount: 11},
		&SuiteNamedCol{ID: "n2", Name: "李四", Amount: 22},
	)
	return drv
}

func suiteCRUD(t *testing.T, f Factory) {
	t.Run("Create 单条与时间戳", func(t *testing.T) {
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteUser{})
		u := &SuiteUser{Name: "alice", Email: "alice@test.dev"}
		if err := drv.Query().Create(u); err != nil {
			t.Fatalf("Create: %v", err)
		}
		// ID 自动生成：16 位非空（§11.3 创建行）
		if len(u.ID) != 16 {
			t.Fatalf("Create 应自动生成 16 位 ID，实际 %q", u.ID)
		}
		// created/updated 双驱动同为 unix 秒
		assertUnixSecond(t, "CreatedAt", u.CreatedAt)
		assertUnixSecond(t, "UpdatedAt", u.UpdatedAt)
		// 落库值读回一致
		var got SuiteUser
		if err := drv.Query().Model(&SuiteUser{}).Where("id = ?", u.ID).Take(&got); err != nil {
			t.Fatalf("读回: %v", err)
		}
		if got.Name != "alice" || got.Score != 0 {
			t.Fatalf("读回不一致: %+v", got)
		}
	})

	t.Run("Create 批量（尾批不足）", func(t *testing.T) {
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteUser{})
		us := []*SuiteUser{
			{Name: "b1", Email: "b1@test.dev"},
			{Name: "b2", Email: "b2@test.dev"},
			{Name: "b3", Email: "b3@test.dev"},
		}
		if err := drv.Query().CreateInBatches(us, 2); err != nil {
			t.Fatalf("CreateInBatches: %v", err)
		}
		var count int64
		if err := drv.Query().Model(&SuiteUser{}).Count(&count); err != nil || count != 3 {
			t.Fatalf("批量应插入 3 行，实际 %d (err=%v)", count, err)
		}
		for i, u := range us {
			if len(u.ID) != 16 || u.CreatedAt == 0 {
				t.Fatalf("第 %d 行 ID/时间戳未填充: %+v", i, u)
			}
		}
	})

	t.Run("Create 显式 ID 不覆盖", func(t *testing.T) {
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteUser{})
		u := &SuiteUser{ID: "explicit00000001", Name: "keep", Email: "keep@test.dev"}
		if err := drv.Query().Create(u); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if u.ID != "explicit00000001" {
			t.Fatalf("显式 ID 不应被覆盖，实际 %q", u.ID)
		}
	})

	t.Run("Find/First/Last/Take", func(t *testing.T) {
		drv := seedUsers(t, f)
		q := drv.Query()

		var all []SuiteUser
		if err := q.Model(&SuiteUser{}).Order("id").Find(&all); err != nil {
			t.Fatalf("Find: %v", err)
		}
		if len(all) != 3 || all[0].ID != "u1" || all[2].ID != "u3" {
			t.Fatalf("Find 全表期望 [u1 u2 u3]，实际 %+v", idsOfUsers(all))
		}

		var first SuiteUser
		if err := q.Model(&SuiteUser{}).First(&first); err != nil || first.ID != "u1" {
			t.Fatalf("First 期望 u1，实际 %s (err=%v)", first.ID, err)
		}
		var last SuiteUser
		if err := q.Model(&SuiteUser{}).Last(&last); err != nil || last.ID != "u3" {
			t.Fatalf("Last 期望 u3，实际 %s (err=%v)", last.ID, err)
		}
		var take SuiteUser
		if err := q.Model(&SuiteUser{}).Take(&take); err != nil || take.ID == "" {
			t.Fatalf("Take 应取到一行，实际 %s (err=%v)", take.ID, err)
		}
	})

	t.Run("空表 First 返回 ErrRecordNotFound", func(t *testing.T) {
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteUser{})
		var u SuiteUser
		err := drv.Query().Model(&SuiteUser{}).First(&u)
		errIs(t, err, contracts.ErrRecordNotFound, "空表 First")
	})

	t.Run("自定义列名查询（'user_name'）", func(t *testing.T) {
		drv := seedUsers(t, f)
		q := drv.Query()
		// Where 命中 patch/mapper 后的列（gorm 侧列名与约定推导 name 不同，
		// 命中即 patch 生效的直接证据，§11.3）
		var got SuiteNamedCol
		if err := q.Model(&SuiteNamedCol{}).Where("user_name = ?", "李四").Take(&got); err != nil {
			t.Fatalf("Where user_name: %v", err)
		}
		if got.ID != "n2" {
			t.Fatalf("Where 'user_name' 期望 n2，实际 %+v", got)
		}
		// Order + Select + Pluck 命中自定义列
		var rows []SuiteNamedCol
		if err := q.Model(&SuiteNamedCol{}).Order("user_name DESC").Find(&rows); err != nil {
			t.Fatalf("Order user_name: %v", err)
		}
		if len(rows) != 2 || rows[0].ID != "n2" {
			t.Fatalf("Order 'user_name' DESC 期望 [n2 n1]，实际 %+v", rows)
		}
		var names []string
		if err := q.Model(&SuiteNamedCol{}).Order("id").Pluck("user_name", &names); err != nil {
			t.Fatalf("Pluck user_name: %v", err)
		}
		if len(names) != 2 || names[0] != "张三" || names[1] != "李四" {
			t.Fatalf("Pluck 'user_name' 期望 [张三 李四]，实际 %v", names)
		}
	})

	t.Run("忽略字段三断言", func(t *testing.T) {
		drv := seedUsers(t, f)
		q := drv.Query()
		// 1) 建表无该列：查询 internal 列必须报错
		var v any
		if err := q.Raw("SELECT internal FROM suite_users LIMIT 1").Scan(&v); err == nil {
			t.Fatal("忽略字段不应建列（SELECT internal 应报错）")
		}
		// 2) 写入不落地 / 3) 读回零值
		u := &SuiteUser{ID: "ign0re000000001", Name: "ign", Email: "ign@test.dev", Internal: "secret"}
		mustCreate(t, q, u)
		var got SuiteUser
		mustTake(t, q, &got, u.ID)
		if got.Internal != "" {
			t.Fatalf("忽略字段读回应为零值，实际 %q", got.Internal)
		}
	})

	t.Run("只写 -> 与只读 <-", func(t *testing.T) {
		drv := seedUsers(t, f)
		q := drv.Query()
		// 只写：Password 落库但读回零值
		var got SuiteUser
		mustTake(t, q, &got, "u1")
		if got.Password != "" {
			t.Fatalf("只写字段读回应为零值，实际 %q", got.Password)
		}
		var pw string
		if err := q.Raw("SELECT password FROM suite_users WHERE id = 'u1'").Scan(&pw); err != nil || pw != "pw-alice" {
			t.Fatalf("只写字段应已落库，实际 %q (err=%v)", pw, err)
		}
		// 只读：Create 不写入（token 列保持 NULL），读回正常填充
		u := &SuiteUser{ID: "readonly00000001", Name: "ro", Email: "ro@test.dev", Token: "ignored"}
		mustCreate(t, q, u)
		var tok sql.NullString
		if err := q.Raw("SELECT token FROM suite_users WHERE id = ?", "readonly00000001").Scan(&tok); err != nil {
			t.Fatalf("只读字段读回: %v", err)
		}
		if tok.Valid {
			t.Fatalf("只读字段 Create 不应写入（token 应为 NULL），实际 %q", tok.String)
		}
		mustExec(t, q, "UPDATE suite_users SET token = 'from-db' WHERE id = ?", "readonly00000001")
		var got2 SuiteUser
		mustTake(t, q, &got2, u.ID)
		if got2.Token != "from-db" {
			t.Fatalf("只读字段应从库读回填充，实际 %q", got2.Token)
		}
	})

	t.Run("乐观锁四断言与并发单方成功", func(t *testing.T) {
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteVersioned{})
		q := drv.Query()

		// 1) Create 置 1
		doc := &SuiteVersioned{ID: "ver1", Title: "v1"}
		if err := q.Create(doc); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if doc.Version != 1 {
			t.Fatalf("插入后 version 应置 1，实际 %d", doc.Version)
		}
		// 2) struct 更新自增并回填
		var got SuiteVersioned
		mustTake(t, q, &got, "ver1")
		got.Title = "v2"
		if err := q.Save(&got); err != nil {
			t.Fatalf("Save(更新): %v", err)
		}
		if got.Version != 2 {
			t.Fatalf("更新后 version 应自增回填 2，实际 %d", got.Version)
		}
		// 3) 陈旧版本更新 0 行不报错
		stale := &SuiteVersioned{ID: "ver1", Title: "stale", Version: 1}
		r := q.Model(&SuiteVersioned{}).Where("id = ?", "ver1").UpdatesResult(stale)
		if r.Error != nil || r.RowsAffected != 0 {
			t.Fatalf("陈旧版本更新应 0 行不报错，实际 %+v", r)
		}
		// 4) Updates(map) 不参与版本控制
		if err := q.Model(&SuiteVersioned{}).Where("id = ?", "ver1").Updates(map[string]any{"title": "mapped"}); err != nil {
			t.Fatalf("Updates(map): %v", err)
		}
		var after SuiteVersioned
		mustTake(t, q, &after, "ver1")
		if after.Version != 2 || after.Title != "mapped" {
			t.Fatalf("map 更新不应改动 version: %+v", after)
		}
		// 并发仅一方成功：两个快照同为 version=2，先到者得
		snapA := &SuiteVersioned{ID: "ver1", Title: "A", Version: 2}
		snapB := &SuiteVersioned{ID: "ver1", Title: "B", Version: 2}
		r1 := q.Model(&SuiteVersioned{}).Where("id = ?", "ver1").UpdatesResult(snapA)
		r2 := q.Model(&SuiteVersioned{}).Where("id = ?", "ver1").UpdatesResult(snapB)
		if r1.Error != nil || r2.Error != nil {
			t.Fatalf("并发更新不应报错: %v / %v", r1.Error, r2.Error)
		}
		successes := int64(0)
		if r1.RowsAffected == 1 {
			successes++
		}
		if r2.RowsAffected == 1 {
			successes++
		}
		if successes != 1 {
			t.Fatalf("并发更新应恰一方成功，实际成功 %d 次（r1=%+v r2=%+v）", successes, r1, r2)
		}
	})

	t.Run("Where/OrWhere/Not 条件", func(t *testing.T) {
		drv := seedUsers(t, f)
		q := drv.Query()

		var rows []SuiteUser
		if err := q.Model(&SuiteUser{}).Where(map[string]any{"dept_id": "d1"}).Order("id").Find(&rows); err != nil {
			t.Fatalf("Where(map): %v", err)
		}
		if len(rows) != 2 || rows[0].ID != "u1" {
			t.Fatalf("Where(map) 期望 [u1 u2]，实际 %v", idsOfUsers(rows))
		}

		rows = nil
		// name = 'alice' OR age = 40
		if err := q.Model(&SuiteUser{}).
			Where("name = ?", "alice").
			OrWhere("age = ?", 40).
			Order("id").Find(&rows); err != nil {
			t.Fatalf("OrWhere: %v", err)
		}
		if len(rows) != 2 || rows[0].ID != "u1" || rows[1].ID != "u2" {
			t.Fatalf("OrWhere 期望 [u1 u2]，实际 %v", idsOfUsers(rows))
		}

		rows = nil
		if err := q.Model(&SuiteUser{}).Not("dept_id = ?", "d1").Find(&rows); err != nil {
			t.Fatalf("Not: %v", err)
		}
		if len(rows) != 1 || rows[0].ID != "u3" {
			t.Fatalf("Not 期望 [u3]，实际 %v", idsOfUsers(rows))
		}
	})

	t.Run("IN 切片展开", func(t *testing.T) {
		drv := seedUsers(t, f)
		q := drv.Query()
		ids := []string{"u1", "u3"}

		assertIDs := func(label string, want []string, err error, rows []SuiteUser) {
			t.Helper()
			if err != nil {
				t.Fatalf("%s: %v", label, err)
			}
			got := idsOfUsers(rows)
			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Fatalf("%s 期望 %v，实际 %v", label, want, got)
			}
		}

		var rows []SuiteUser
		err := q.Model(&SuiteUser{}).Where("id IN ?", ids).Order("id").Find(&rows)
		assertIDs("IN ?", []string{"u1", "u3"}, err, rows)

		rows = nil
		err = q.Model(&SuiteUser{}).Where("id IN (?)", ids).Order("id").Find(&rows)
		assertIDs("IN (?)", []string{"u1", "u3"}, err, rows)

		rows = nil
		err = q.Model(&SuiteUser{}).Where("id NOT IN ?", ids).Order("id").Find(&rows)
		assertIDs("NOT IN ?", []string{"u2"}, err, rows)

		rows = nil
		err = q.Model(&SuiteUser{}).Where("id IN ?", []string{}).Find(&rows)
		assertIDs("空切片恒假", nil, err, rows)

		rows = nil
		err = q.Model(&SuiteUser{}).Where("dept_id = ? AND id IN ?", "d1", ids).Find(&rows)
		assertIDs("混合占位符", []string{"u1"}, err, rows)
	})

	t.Run("Update 单列零值写入 / Updates map 全键 / Updates struct 零值跳过", func(t *testing.T) {
		drv := seedUsers(t, f)
		q := drv.Query()

		// struct：零值字段跳过——u1.age=30 应保持不变，仅 name 写入
		if err := q.Model(&SuiteUser{}).Where("id = ?", "u1").Updates(&SuiteUser{Name: "alice2"}); err != nil {
			t.Fatalf("Updates(struct): %v", err)
		}
		var u SuiteUser
		mustTake(t, q, &u, "u1")
		if u.Name != "alice2" {
			t.Fatalf("Updates(struct) 非零字段应写入，实际 %+v", u)
		}
		if u.Age != 30 {
			t.Fatalf("Updates(struct) 零值字段应跳过（age 应保持 30），实际 %d", u.Age)
		}

		// 单列：显式指定列（含零值）无条件写入
		if err := q.Model(&SuiteUser{}).Where("id = ?", "u1").Update("age", 0); err != nil {
			t.Fatalf("Update(0): %v", err)
		}
		mustTake(t, q, &u, "u1")
		if u.Age != 0 {
			t.Fatalf("Update 单列零值应写入，实际 age=%d", u.Age)
		}

		// map：全键写入（含零值）
		if err := q.Model(&SuiteUser{}).Where("id = ?", "u2").Updates(map[string]any{"age": 0, "score": 0.0}); err != nil {
			t.Fatalf("Updates(map): %v", err)
		}
		mustTake(t, q, &u, "u2")
		if u.Age != 0 || u.Score != 0 {
			t.Fatalf("Updates(map) 全键应写入，实际 %+v", u)
		}
	})

	t.Run("Save 三分支", func(t *testing.T) {
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteUser{})
		q := drv.Query()

		// 空主键：插入
		ins := &SuiteUser{Name: "saved-insert", Email: "si@test.dev"}
		if err := q.Save(ins); err != nil {
			t.Fatalf("Save(插入分支): %v", err)
		}
		if len(ins.ID) != 16 {
			t.Fatalf("Save 插入分支应生成 ID，实际 %q", ins.ID)
		}
		// 非空主键：更新
		ins.Name = "saved-update"
		if err := q.Save(ins); err != nil {
			t.Fatalf("Save(更新分支): %v", err)
		}
		var got SuiteUser
		mustTake(t, q, &got, ins.ID)
		if got.Name != "saved-update" {
			t.Fatalf("Save 更新分支未落库: %+v", got)
		}
		// 更新 0 行回落 INSERT（upsert）
		upsert := &SuiteUser{ID: "upsert16000000001", Name: "upsert", Email: "up@test.dev"}
		if err := q.Save(upsert); err != nil {
			t.Fatalf("Save(回落插入): %v", err)
		}
		var count int64
		if err := q.Model(&SuiteUser{}).Where("id = ?", upsert.ID).Count(&count); err != nil || count != 1 {
			t.Fatalf("Save 0 行回落应插入 1 行，实际 %d (err=%v)", count, err)
		}
	})

	t.Run("Delete 行数", func(t *testing.T) {
		drv := seedUsers(t, f)
		q := drv.Query()
		r := q.DeleteResult(&SuiteUser{}, "id = ?", "u1")
		if r.Error != nil || r.RowsAffected != 1 {
			t.Fatalf("DeleteResult 期望 1 行，实际 %+v", r)
		}
		var count int64
		if err := q.Model(&SuiteUser{}).Count(&count); err != nil || count != 2 {
			t.Fatalf("删除后应剩 2 行，实际 %d (err=%v)", count, err)
		}
	})

	t.Run("Count/Exists/Pluck", func(t *testing.T) {
		drv := seedUsers(t, f)
		q := drv.Query()

		var count int64
		// 带排序的 Count：ORDER BY 应被剥离（§11.3）
		if err := q.Model(&SuiteUser{}).Order("id").Count(&count); err != nil || count != 3 {
			t.Fatalf("Count 期望 3，实际 %d (err=%v)", count, err)
		}

		exists, err := q.Exists(&SuiteUser{})
		if err != nil || !exists {
			t.Fatalf("Exists(有数据) 期望 true，实际 %v (err=%v)", exists, err)
		}
		exists, err = q.Exists(&SuiteUser{}, "id = ?", "nope")
		if err != nil || exists {
			t.Fatalf("Exists(未命中) 期望 false，实际 %v (err=%v)", exists, err)
		}

		var depts []string
		if err := q.Model(&SuiteUser{}).Order("id").Pluck("dept_id", &depts); err != nil {
			t.Fatalf("Pluck: %v", err)
		}
		if strings.Join(depts, ",") != "d1,d1,d2" {
			t.Fatalf("Pluck(dept_id) 期望 [d1 d1 d2]，实际 %v", depts)
		}
		// Pluck 类型不匹配 → 报错（信息含列名，§11.3）
		var bad []int
		if err := q.Model(&SuiteUser{}).Pluck("name", &bad); err == nil {
			t.Fatal("Pluck(name → []int) 应报类型错误")
		}
		// TODO(11.3)：Pluck 含 NULL 行（gorm nil 填充 / xorm 报错——差异固化，见 §11.14 备注）
	})

	t.Run("Paginate 边界", func(t *testing.T) {
		drv := seedUsers(t, f)
		q := drv.Query()

		var rows []SuiteUser
		if err := q.Model(&SuiteUser{}).Order("id").Paginate(2, 2).Find(&rows); err != nil {
			t.Fatalf("Paginate(2,2): %v", err)
		}
		if len(rows) != 1 || rows[0].ID != "u3" {
			t.Fatalf("Paginate(2,2) 期望第 3 行 u3，实际 %v", idsOfUsers(rows))
		}
		// 非法入参归一化（utils.PageUtil）：不 panic 且结果合法
		rows = nil
		if err := q.Model(&SuiteUser{}).Order("id").Paginate(0, 0).Find(&rows); err != nil {
			t.Fatalf("Paginate(0,0): %v", err)
		}
		if len(rows) != 3 {
			t.Fatalf("Paginate(0,0) 归一化后应返回首页全量，实际 %d", len(rows))
		}
		rows = nil
		if err := q.Model(&SuiteUser{}).Paginate(-1, -5).Find(&rows); err != nil || len(rows) != 3 {
			t.Fatalf("Paginate(-1,-5) 归一化应返回全量，实际 %d (err=%v)", len(rows), err)
		}
		rows = nil
		if err := q.Model(&SuiteUser{}).Paginate(99, 2).Find(&rows); err != nil || len(rows) != 0 {
			t.Fatalf("超大页码应返回空集，实际 %d (err=%v)", len(rows), err)
		}
	})

	t.Run("Omit/Distinct/Scopes", func(t *testing.T) {
		drv := seedUsers(t, f)
		q := drv.Query()

		// Omit：查询投影排除列（读回零值，库中值不受影响）
		var u SuiteUser
		if err := q.Model(&SuiteUser{}).Omit("age").Where("id = ?", "u1").Take(&u); err != nil {
			t.Fatalf("Omit Take: %v", err)
		}
		if u.Age != 0 || u.Name != "alice" {
			t.Fatalf("Omit(age) 后 age 应为读回零值，实际 %+v", u)
		}

		// Distinct：去重列投影
		var rows []struct {
			DeptID string `orm:"'dept_id'"`
		}
		if err := q.Model(&SuiteUser{}).Distinct("dept_id").Order("dept_id").Scan(&rows); err != nil {
			t.Fatalf("Distinct Scan: %v", err)
		}
		if len(rows) != 2 || rows[0].DeptID != "d1" || rows[1].DeptID != "d2" {
			t.Fatalf("Distinct(dept_id) 期望 [d1 d2]，实际 %+v", rows)
		}

		// Scopes：作用域组合（nil 跳过为 xorm 侧行为，gorm 侧不支持——不纳入公共断言）
		rows = nil
		scopeDept := func(qq contracts.Query) contracts.Query { return qq.Where("dept_id = ?", "d1") }
		scopeOrder := func(qq contracts.Query) contracts.Query { return qq.Order("id DESC") }
		if err := q.Model(&SuiteUser{}).Scopes(scopeDept, scopeOrder).Scan(&rows); err != nil {
			t.Fatalf("Scopes: %v", err)
		}
		if len(rows) != 2 || rows[0].DeptID != "d1" {
			t.Fatalf("Scopes 组合期望 d1 两行倒序，实际 %+v", rows)
		}
		// TODO(11.3)：Group+Having / Select("*") 回归 / Debug()——见文档 §11.3 其他行
	})

	// 时间戳推进防抖：确保 seeded 行的 updated_at 早于后续写操作（软删/缓存用例复用）
	t.Run("created 与 updated 填充一致性", func(t *testing.T) {
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteUser{})
		u := &SuiteUser{Name: "ts", Email: "ts@test.dev"}
		if err := drv.Query().Create(u); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if u.CreatedAt == 0 || u.UpdatedAt == 0 {
			t.Fatalf("created/updated 均应填充: %+v", u)
		}
		diff := u.UpdatedAt - u.CreatedAt
		if diff < -2 || diff > 2 {
			t.Fatalf("同一事务内 created/updated 应几乎同期（unix 秒），实际 %d/%d", u.CreatedAt, u.UpdatedAt)
		}
		if time.Now().Unix()-u.CreatedAt > 120 {
			t.Fatalf("created 应为当前时间附近，实际 %d", u.CreatedAt)
		}
	})
}

// idsOfUsers 提取用户 ID 列表（断言辅助）。
func idsOfUsers(rows []SuiteUser) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out
}
