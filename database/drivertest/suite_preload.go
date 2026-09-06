// suite_preload.go 预加载一致性矩阵（orm-tag-design.md §11.5 全核心行）。
// 双驱动路径不同（gorm 原生 / 共享引擎），最终回填结果必须逐项一致；
// 非 N+1 以可选 QueryCounter 断言（未实现则 t.Log 跳过）。

package drivertest

import (
	"strings"
	"testing"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// seedRelFixtures 建关联全景图：
//
//	depts:   d1(研发) d2(运维)
//	users:   u1(d1) u2(d1) u3(DeptID="" 零值外键)
//	profile: p1→u1（u2 无 profile；u3 无 profile）
//	orders:  o1(u1,100) o2(u1,250) o3(u2,50)（u3 无订单）
//	items:   i1,i2→o1（o2 无 items）；i3→o3
//	roles:   r1(c1,admin) r2(c2,dev) r3(c3,guest)；中间表 u1→[c1,c2] u2→[c2] u3→无
//	photos:  ph1(u1,suite_users) ph2(u2,suite_users) ph3(o1,suite_orders)
//	         ph4(o2,suite_orders) ph5(u1,other_places 异类型) ph6(r1,suite_roles 缺省值)
//	nodes:   n1→(n2,n3)→n4 子于 n2；n5 孤立
//	compounds: cp1(t1)→cc1(cp1,t1)；cp2(t2)→cc2(cp2,t2)；cc3(cp1,t2) 无主
func seedRelFixtures(t *testing.T, f Factory) contracts.Driver {
	t.Helper()
	drv := f(t)
	mustAutoMigrate(t, drv,
		&SuiteUser{}, &SuiteOrder{}, &SuiteOrderItem{}, &SuiteProfile{}, &SuiteDept{},
		&SuiteRole{}, &SuitePhoto{}, &SuiteUserRole{}, &SuiteNode{},
		&SuiteCompound{}, &SuiteCompoundChild{},
	)
	q := drv.Query()

	mustCreate(t, q,
		&SuiteDept{ID: "d1", Name: "研发"},
		&SuiteDept{ID: "d2", Name: "运维"},
		&SuiteUser{ID: "u1", Name: "alice", Email: "alice@test.dev", DeptID: "d1"},
		&SuiteUser{ID: "u2", Name: "bob", Email: "bob@test.dev", DeptID: "d1"},
		&SuiteUser{ID: "u3", Name: "carol", Email: "carol@test.dev"},
		&SuiteProfile{ID: "p1", SuiteUserID: "u1", Bio: "alice 的资料"},
		&SuiteOrder{ID: "o1", SuiteUserID: "u1", Amount: 100},
		&SuiteOrder{ID: "o2", SuiteUserID: "u1", Amount: 250},
		&SuiteOrder{ID: "o3", SuiteUserID: "u2", Amount: 50},
		&SuiteOrderItem{ID: "i1", SuiteOrderID: "o1", Sku: "book"},
		&SuiteOrderItem{ID: "i2", SuiteOrderID: "o1", Sku: "pen"},
		&SuiteOrderItem{ID: "i3", SuiteOrderID: "o3", Sku: "bag"},
		&SuiteRole{ID: "r1", Code: "c1", Name: "admin"},
		&SuiteRole{ID: "r2", Code: "c2", Name: "dev"},
		&SuiteRole{ID: "r3", Code: "c3", Name: "guest"},
		&SuiteNode{ID: "n1", Name: "root"},
		&SuiteNode{ID: "n2", Name: "left", ParentID: "n1"},
		&SuiteNode{ID: "n3", Name: "right", ParentID: "n1"},
		&SuiteNode{ID: "n4", Name: "leaf", ParentID: "n2"},
		&SuiteNode{ID: "n5", Name: "lonely"},
		&SuiteCompound{ID: "cp1", TenantID: "t1", Name: "cp1"},
		&SuiteCompound{ID: "cp2", TenantID: "t2", Name: "cp2"},
		&SuiteCompoundChild{ID: "cc1", OrgID: "cp1", DeptID: "t1", Label: "cp1-t1"},
		&SuiteCompoundChild{ID: "cc2", OrgID: "cp2", DeptID: "t2", Label: "cp2-t2"},
		&SuiteCompoundChild{ID: "cc3", OrgID: "cp1", DeptID: "t2", Label: "orphan"},
		&SuitePhoto{ID: "ph1", OwnerID: "u1", OwnerType: "suite_users", URL: "u1.png"},
		&SuitePhoto{ID: "ph2", OwnerID: "u2", OwnerType: "suite_users", URL: "u2.png"},
		&SuitePhoto{ID: "ph3", OwnerID: "o1", OwnerType: "suite_orders", URL: "o1.png"},
		&SuitePhoto{ID: "ph4", OwnerID: "o2", OwnerType: "suite_orders", URL: "o2.png"},
		&SuitePhoto{ID: "ph5", OwnerID: "u1", OwnerType: "other_places", URL: "x.png"},
		&SuitePhoto{ID: "ph6", OwnerID: "r1", OwnerType: "suite_roles", URL: "r1.png"},
	)
	// many2many 中间表（业务显式写入，引擎只读，§9.2 契约 9）
	mustExec(t, q,
		"INSERT INTO suite_user_roles (id, code) VALUES ('u1','c1'), ('u1','c2'), ('u2','c2')",
	)
	return drv
}

// assertQueryCount 计数断言：drv 实现 QueryCounter 时校验 fn 期间的 SQL 次数
// ≤ max；未实现时记录日志跳过（§11.5 尽力而为）。
func assertQueryCount(t *testing.T, drv contracts.Driver, max int, label string, fn func()) {
	t.Helper()
	count, ok := queryCountOf(drv)
	if !ok {
		t.Logf("驱动未实现 QueryCounter，跳过 %s 的 SQL 计数断言", label)
		fn()
		return
	}
	before := count()
	fn()
	n := count() - before
	if n > max {
		t.Fatalf("%s: 期望 SQL 查询次数 ≤%d（批量 IN 非 N+1），实际 %d", label, max, n)
	}
}

func suitePreload(t *testing.T, f Factory) {
	t.Run("has-many 基础与非 N+1", func(t *testing.T) {
		drv := seedRelFixtures(t, f)
		q := drv.Query()
		var rows []SuiteUser
		assertQueryCount(t, drv, 2, "has-many Preload", func() {
			if err := q.Model(&SuiteUser{}).Order("id").Preload("Orders").Find(&rows); err != nil {
				t.Fatalf("Preload(Orders): %v", err)
			}
		})
		if len(rows) != 3 {
			t.Fatalf("3 父行，实际 %d", len(rows))
		}
		if len(rows[0].Orders) != 2 || rows[0].Orders[0].ID != "o1" || rows[0].Orders[1].ID != "o2" {
			t.Fatalf("u1.Orders 期望 [o1 o2]，实际 %+v", rows[0].Orders)
		}
		if len(rows[1].Orders) != 1 || rows[1].Orders[0].ID != "o3" {
			t.Fatalf("u2.Orders 期望 [o3]，实际 %+v", rows[1].Orders)
		}
	})

	t.Run("has-many 空结果回填非 nil 空切片", func(t *testing.T) {
		drv := seedRelFixtures(t, f)
		var rows []SuiteUser
		if err := drv.Query().Model(&SuiteUser{}).Where("id = ?", "u3").Preload("Orders").Find(&rows); err != nil {
			t.Fatalf("Preload: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("u3 应存在，实际 %d", len(rows))
		}
		if rows[0].Orders == nil || len(rows[0].Orders) != 0 {
			t.Fatalf("无子行 Orders 应为非 nil 空切片，实际 %#v", rows[0].Orders)
		}
	})

	t.Run("has-one 无子行保持 nil", func(t *testing.T) {
		drv := seedRelFixtures(t, f)
		var rows []SuiteUser
		if err := drv.Query().Model(&SuiteUser{}).Order("id").Preload("Profile").Find(&rows); err != nil {
			t.Fatalf("Preload(Profile): %v", err)
		}
		if rows[0].Profile == nil || rows[0].Profile.Bio != "alice 的资料" {
			t.Fatalf("u1.Profile 期望回填，实际 %+v", rows[0].Profile)
		}
		if rows[1].Profile != nil {
			t.Fatalf("u2 无 profile 应保持 nil，实际 %+v", rows[1].Profile)
		}
	})

	t.Run("belongs-to 方向与回填", func(t *testing.T) {
		drv := seedRelFixtures(t, f)
		var rows []SuiteUser
		if err := drv.Query().Model(&SuiteUser{}).Order("id").Preload("Dept").Find(&rows); err != nil {
			t.Fatalf("Preload(Dept): %v", err)
		}
		if rows[0].Dept == nil || rows[0].Dept.Name != "研发" {
			t.Fatalf("u1.Dept 期望 研发，实际 %+v", rows[0].Dept)
		}
		if rows[2].Dept != nil {
			t.Fatalf("u3 零值外键 Dept 应保持 nil（不报错），实际 %+v", rows[2].Dept)
		}
	})

	t.Run("嵌套 Orders.Items", func(t *testing.T) {
		drv := seedRelFixtures(t, f)
		var rows []SuiteUser
		if err := drv.Query().Model(&SuiteUser{}).Order("id").Preload("Orders.Items").Find(&rows); err != nil {
			t.Fatalf("Preload(Orders.Items): %v", err)
		}
		if len(rows[0].Orders) != 2 {
			t.Fatalf("u1.Orders 期望 2，实际 %+v", rows[0].Orders)
		}
		byID := map[string]*SuiteOrder{}
		for i := range rows[0].Orders {
			byID[rows[0].Orders[i].ID] = &rows[0].Orders[i]
		}
		if len(byID["o1"].Items) != 2 {
			t.Fatalf("o1.Items 期望 2，实际 %+v", byID["o1"].Items)
		}
		if byID["o2"].Items == nil || len(byID["o2"].Items) != 0 {
			t.Fatalf("o2 无 items 应为非 nil 空切片，实际 %#v", byID["o2"].Items)
		}
	})

	t.Run("conds 条件过滤", func(t *testing.T) {
		drv := seedRelFixtures(t, f)
		var rows []SuiteUser
		if err := drv.Query().Model(&SuiteUser{}).Order("id").Preload("Orders", "amount > ?", 100).Find(&rows); err != nil {
			t.Fatalf("Preload conds: %v", err)
		}
		if len(rows[0].Orders) != 1 || rows[0].Orders[0].ID != "o2" {
			t.Fatalf("conds 应仅回填 o2（amount>100），实际 %+v", rows[0].Orders)
		}
		if rows[1].Orders == nil || len(rows[1].Orders) != 0 {
			t.Fatalf("u2 无命中子行应为空切片，实际 %#v", rows[1].Orders)
		}
	})

	t.Run("callback 排序限量", func(t *testing.T) {
		drv := seedRelFixtures(t, f)
		var rows []SuiteUser
		err := drv.Query().Model(&SuiteUser{}).Order("id").
			Preload("Roles", func(cq contracts.Query) contracts.Query {
				return cq.Order("name DESC").Limit(1)
			}).Find(&rows)
		if err != nil {
			t.Fatalf("Preload callback: %v", err)
		}
		// u1 roles=[admin(c1),dev(c2)]，name DESC → dev(c2) 限量 1
		if len(rows[0].Roles) != 1 || rows[0].Roles[0].Code != "c2" {
			t.Fatalf("callback 排序限量期望 [c2]，实际 %+v", roleCodes(rows[0].Roles))
		}
		// 注：gorm 原生 Preload 路径不接受 contracts.Query 回调（gorm:"-" 引擎
		// 字段双驱动一致走共享引擎），故回调用例选用引擎字段 Roles。
	})

	t.Run("conds 与 callback 共存", func(t *testing.T) {
		drv := seedRelFixtures(t, f)
		var rows []SuiteUser
		err := drv.Query().Model(&SuiteUser{}).Order("id").
			Preload("Roles", "code != ?", "c1", func(cq contracts.Query) contracts.Query {
				return cq.Order("code ASC").Limit(1)
			}).Find(&rows)
		if err != nil {
			t.Fatalf("Preload conds+callback: %v", err)
		}
		// u1: conds 过滤掉 c1 → [c2]，callback 限量 1
		if len(rows[0].Roles) != 1 || rows[0].Roles[0].Code != "c2" {
			t.Fatalf("conds+callback 期望 [c2]，实际 %+v", roleCodes(rows[0].Roles))
		}
		if len(rows[1].Roles) != 1 || rows[1].Roles[0].Code != "c2" {
			t.Fatalf("conds+callback u2 期望 [c2]，实际 %+v", roleCodes(rows[1].Roles))
		}
	})

	t.Run("复合外键元组", func(t *testing.T) {
		drv := seedRelFixtures(t, f)
		var rows []SuiteCompound
		if err := drv.Query().Model(&SuiteCompound{}).Order("id").Preload("Parent").Find(&rows); err != nil {
			t.Fatalf("Preload(Parent): %v", err)
		}
		if len(rows) != 2 {
			t.Fatalf("2 父行，实际 %d", len(rows))
		}
		if rows[0].Parent == nil || rows[0].Parent.ID != "cc1" {
			t.Fatalf("cp1.Parent 期望 cc1，实际 %+v", rows[0].Parent)
		}
		if rows[1].Parent == nil || rows[1].Parent.ID != "cc2" {
			t.Fatalf("cp2.Parent 期望 cc2（元组不错位），实际 %+v", rows[1].Parent)
		}
	})

	t.Run("自引用树不成环", func(t *testing.T) {
		drv := seedRelFixtures(t, f)
		var rows []SuiteNode
		if err := drv.Query().Model(&SuiteNode{}).Where("parent_id IS NULL OR parent_id = ''").
			Order("id").Preload("Children").Find(&rows); err != nil {
			t.Fatalf("Preload(Children): %v", err)
		}
		if len(rows) != 2 { // n1 + n5
			t.Fatalf("根节点期望 2（n1/n5），实际 %d", len(rows))
		}
		var root *SuiteNode
		for i := range rows {
			if rows[i].ID == "n1" {
				root = &rows[i]
			}
		}
		if root == nil || len(root.Children) != 2 {
			t.Fatalf("n1.Children 期望 2，实际 %+v", root)
		}
		var lonely *SuiteNode
		for i := range rows {
			if rows[i].ID == "n5" {
				lonely = &rows[i]
			}
		}
		if lonely.Children == nil || len(lonely.Children) != 0 {
			t.Fatalf("n5.Children 应为非 nil 空切片，实际 %#v", lonely.Children)
		}
	})

	t.Run("终结方法覆盖与 FindInBatches 逐批", func(t *testing.T) {
		drv := seedRelFixtures(t, f)
		q := drv.Query()

		var one SuiteUser
		if err := q.Model(&SuiteUser{}).Order("id").Preload("Orders").First(&one); err != nil {
			t.Fatalf("First + Preload: %v", err)
		}
		if len(one.Orders) != 2 {
			t.Fatalf("First 回填期望 2 子行，实际 %+v", one.Orders)
		}
		var take SuiteUser
		// Last 自带主键降序排序：链上显式 Order 会与之叠加（gorm 侧 ORDER BY
		// id, id DESC 使首行胜出），故此处不加链上排序，锁定 Last 语义本身。
		if err := q.Model(&SuiteUser{}).Preload("Orders").Last(&take); err != nil {
			t.Fatalf("Last + Preload: %v", err)
		}
		if take.ID != "u3" || take.Orders == nil || len(take.Orders) != 0 {
			t.Fatalf("Last(u3) 回填期望空切片，实际 %+v", take.Orders)
		}

		batches := 0
		var all []SuiteUser
		err := q.Model(&SuiteUser{}).Order("id").Preload("Orders").FindInBatches(&all, 2, func(tx contracts.Query, batch int) error {
			batches++
			for _, u := range all {
				want := map[string]int{"u1": 2, "u2": 1, "u3": 0}[u.ID]
				if len(u.Orders) != want {
					t.Errorf("批内 %s.Orders 期望 %d（fc 回调时已就绪），实际 %d", u.ID, want, len(u.Orders))
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("FindInBatches + Preload: %v", err)
		}
		if batches != 2 {
			t.Fatalf("3 父行按 2 分批应 2 批，实际 %d", batches)
		}
	})

	t.Run("many2many 多父行不错位", func(t *testing.T) {
		drv := seedRelFixtures(t, f)
		q := drv.Query()
		var rows []SuiteUser
		assertQueryCount(t, drv, 3, "many2many Preload", func() {
			if err := q.Model(&SuiteUser{}).Order("id").Preload("Roles").Find(&rows); err != nil {
				t.Fatalf("Preload(Roles): %v", err)
			}
		})
		// joinForeignKey:ID / joinReferences:Code 为非主键连接键（Code），映射正确
		if len(rows[0].Roles) != 2 || rows[0].Roles[0].Code != "c1" || rows[0].Roles[1].Code != "c2" {
			t.Fatalf("u1.Roles 期望 [c1 c2]，实际 %+v", roleCodes(rows[0].Roles))
		}
		if len(rows[1].Roles) != 1 || rows[1].Roles[0].Code != "c2" {
			t.Fatalf("u2.Roles 期望 [c2]，实际 %+v", roleCodes(rows[1].Roles))
		}
		// 中间表无行 → 回填非 nil 空切片
		if rows[2].Roles == nil || len(rows[2].Roles) != 0 {
			t.Fatalf("u3.Roles 应为非 nil 空切片，实际 %#v", rows[2].Roles)
		}
	})

	t.Run("many2many conds 作用于子表", func(t *testing.T) {
		drv := seedRelFixtures(t, f)
		var rows []SuiteUser
		err := drv.Query().Model(&SuiteUser{}).Order("id").
			Preload("Roles", "name = ?", "dev").Find(&rows)
		if err != nil {
			t.Fatalf("Preload(Roles conds): %v", err)
		}
		if len(rows[0].Roles) != 1 || rows[0].Roles[0].Code != "c2" {
			t.Fatalf("conds 过滤期望 u1 仅 [c2]，实际 %+v", roleCodes(rows[0].Roles))
		}
		if len(rows[1].Roles) != 1 || rows[1].Roles[0].Code != "c2" {
			t.Fatalf("conds 过滤期望 u2 [c2]，实际 %+v", roleCodes(rows[1].Roles))
		}
	})

	t.Run("polymorphic 显式值与类型隔离", func(t *testing.T) {
		drv := seedRelFixtures(t, f)
		q := drv.Query()
		var rows []SuiteUser
		if err := q.Model(&SuiteUser{}).Order("id").Preload("Photos").Find(&rows); err != nil {
			t.Fatalf("Preload(Photos): %v", err)
		}
		// owner_type='suite_users'：u1 仅 ph1（ph5 异类型 other_places 被隔离）
		if len(rows[0].Photos) != 1 || rows[0].Photos[0].URL != "u1.png" {
			t.Fatalf("u1.Photos 期望 [u1.png]，实际 %+v", photoURLs(rows[0].Photos))
		}
		if len(rows[1].Photos) != 1 || rows[1].Photos[0].URL != "u2.png" {
			t.Fatalf("u2.Photos 期望 [u2.png]，实际 %+v", photoURLs(rows[1].Photos))
		}
		// SuiteOrder.Photos（polymorphicValue=suite_orders）：与用户照片不串
		var orders []SuiteOrder
		if err := q.Model(&SuiteOrder{}).Order("id").Preload("Photos").Find(&orders); err != nil {
			t.Fatalf("Preload(Order.Photos): %v", err)
		}
		if len(orders[0].Photos) != 1 || orders[0].Photos[0].URL != "o1.png" {
			t.Fatalf("o1.Photos 期望 [o1.png]，实际 %+v", photoURLs(orders[0].Photos))
		}
	})

	t.Run("polymorphic 缺省值回退父表名", func(t *testing.T) {
		drv := seedRelFixtures(t, f)
		var rows []SuiteRole
		if err := drv.Query().Model(&SuiteRole{}).Order("id").Preload("Photos").Find(&rows); err != nil {
			t.Fatalf("Preload(Role.Photos): %v", err)
		}
		// SuiteRole.Photos 无 polymorphicValue → 缺省回退父表名 suite_roles
		if len(rows[0].Photos) != 1 || rows[0].Photos[0].URL != "r1.png" {
			t.Fatalf("r1.Photos 期望 [r1.png]（缺省类型值 suite_roles），实际 %+v", photoURLs(rows[0].Photos))
		}
		if rows[1].Photos == nil || len(rows[1].Photos) != 0 {
			t.Fatalf("r2.Photos 应为非 nil 空切片，实际 %#v", rows[1].Photos)
		}
	})

	t.Run("[]*T 形态回填", func(t *testing.T) {
		drv := seedRelFixtures(t, f)
		var rows []*SuiteUser
		if err := drv.Query().Model(&SuiteUser{}).Order("id").Preload("Orders").Find(&rows); err != nil {
			t.Fatalf("[]*T Preload: %v", err)
		}
		if len(rows) != 3 || len(rows[0].Orders) != 2 || len(rows[2].Orders) != 0 {
			t.Fatalf("[]*T 形态回填异常: %+v", rows)
		}
	})

	t.Run("错误路径：rel 外键字段不存在", func(t *testing.T) {
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteBrokenRel{}, &SuiteNode{})
		q := drv.Query()
		mustCreate(t, q, &SuiteBrokenRel{ID: "brk000000000001", Title: "broken"})
		var rows []SuiteBrokenRel
		err := q.Model(&SuiteBrokenRel{}).Preload("Nodes").Find(&rows)
		errIs(t, err, contracts.ErrUnsupported, "rel 外键不存在")
	})

	t.Run("错误路径：关联字段无忽略标记（差异固化）", func(t *testing.T) {
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteUntaggedRel{}, &SuiteUntaggedOrder{})
		q := drv.Query()
		mustCreate(t, q, &SuiteUntaggedRel{ID: "untag00000000001", Title: "untagged"})
		var rows []SuiteUntaggedRel
		err := q.Model(&SuiteUntaggedRel{}).Preload("Orders").Find(&rows)
		// 差异固化（§11.5 错误路径行）：xorm 共享引擎前置校验报 ErrUnsupported；
		// gorm 约定外键走原生 Preload 无感成功。以 DriverName 分支固化。
		if strings.Contains(drv.DriverName(), "xorm") {
			errIs(t, err, contracts.ErrUnsupported, "xorm 无忽略标记")
		} else if err != nil {
			t.Fatalf("gorm 约定外键原生 Preload 应成功，实际 %v", err)
		}
	})

	// TODO(11.5)：以下行为本轮未纳入，见文档对应行——
	//   - 缓存路径 Preload（Cache() 命中后仍执行）：依赖 QueryCache 启用，见 suiteCache；
	//   - many2many 嵌套 Roles.Permissions：需第四层模型；
	//   - gorm 双路径一致（同一关联「原生」vs「gorm:"-" + rel + 引擎」）：由
	//     gofast-gorm/preload_engine_test.go 在驱动侧锁定。
}

func roleCodes(rows []SuiteRole) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Code)
	}
	return out
}

func photoURLs(rows []SuitePhoto) []string {
	out := make([]string, 0, len(rows))
	for _, p := range rows {
		out = append(out, p.URL)
	}
	return out
}
