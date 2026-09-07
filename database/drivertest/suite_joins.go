// suite_joins.go Joins 一致性组（orm-tag-design.md §11.4 / 双驱动测试方案 §六
// 第 7 项 Q-08 的 SQLite 子集）。覆盖：
//   - 缺省 INNER JOIN（无前缀关键字）行集与列值；
//   - LEFT JOIN 无匹配补 NULL 行（NULL 形态宽松断言，抹平驱动归一差异）；
//   - 带参 ON（两驱动的 Joins 变参均按序填充 ON 条件中的 ? 占位符：gorm 经
//     clause.Expr / xorm 经 s.Join(op, table, cond, args...)，语义一致，可断言）；
//   - 自连接别名（同表 JOIN 查上级）；
//   - 非法 JOIN 串报错——差异固化在驱动侧（gorm 原样透传 → 数据库语法错误；
//     xorm 解析失败 → ErrUnsupported 包装），套件内只断言「报错」不锁哨兵。
//
// 联表查询统一走显式 Select 投影 + ScanMap 回收（别名 uid/amount），数值经
// suiteMapNum 归一后比较（与驱动侧 fullcov ext Joins 用例同口径）。

package drivertest

import (
	"testing"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// seedSuiteJoinUsers 建 SuiteUser×3 + SuiteOrder×3：
// ju1 两单（100/250）、ju2 一单（50）、ju3 无订单（LEFT JOIN NULL 形态断言用）。
func seedSuiteJoinUsers(t *testing.T, f Factory) contracts.Driver {
	t.Helper()
	drv := f(t)
	mustAutoMigrate(t, drv, &SuiteUser{}, &SuiteOrder{})
	q := drv.Query()
	mustCreate(t, q,
		&SuiteUser{ID: "ju1", Name: "alice", Email: "ju1@test.dev", DeptID: "d1"},
		&SuiteUser{ID: "ju2", Name: "bob", Email: "ju2@test.dev", DeptID: "d1"},
		&SuiteUser{ID: "ju3", Name: "carol", Email: "ju3@test.dev", DeptID: "d2"},
		&SuiteOrder{ID: "jo1", SuiteUserID: "ju1", Amount: 100},
		&SuiteOrder{ID: "jo2", SuiteUserID: "ju1", Amount: 250},
		&SuiteOrder{ID: "jo3", SuiteUserID: "ju2", Amount: 50},
	)
	return drv
}

// joinAmountsByUser 执行联表链并按 uid 归集 amount 列表（同用户多行不折叠）。
func joinAmountsByUser(t *testing.T, chain contracts.Query) map[string][]int64 {
	t.Helper()
	var rows []map[string]any
	if err := chain.ScanMap(&rows); err != nil {
		t.Fatalf("联表 ScanMap: %v", err)
	}
	out := make(map[string][]int64, len(rows))
	for _, m := range rows {
		uid := suiteMapStr(m["uid"])
		out[uid] = append(out[uid], suiteMapNum(t, "amount", m["amount"]))
	}
	return out
}

// assertAmounts 断言某 uid 的金额集合恰为期望值（数量与值全等）。
func assertAmounts(t *testing.T, got map[string][]int64, uid string, want ...int64) {
	t.Helper()
	list := got[uid]
	if len(list) != len(want) {
		t.Fatalf("uid %s 金额期望 %v，实际 %v（全量 %v）", uid, want, list, got)
	}
	for i := range want {
		if list[i] != want[i] {
			t.Fatalf("uid %s 金额期望 %v，实际 %v（全量 %v）", uid, want, list, got)
		}
	}
}

func suiteJoins(t *testing.T, f Factory) {
	t.Run("缺省 INNER JOIN 行集与列值", func(t *testing.T) {
		drv := seedSuiteJoinUsers(t, f)
		q := drv.Query()

		// 无前缀关键字按 INNER 处理：ju3 无订单 → 不产生行
		inner := joinAmountsByUser(t, q.Table("suite_users").
			Select("suite_users.id AS uid", "suite_orders.amount").
			Joins("JOIN suite_orders ON suite_orders.suite_user_id = suite_users.id").
			Order("suite_users.id"))
		if _, dup := inner["ju3"]; dup {
			t.Fatalf("缺省 INNER JOIN 不应含无订单用户 ju3，实际 %v", inner)
		}
		assertAmounts(t, inner, "ju1", 100, 250)
		assertAmounts(t, inner, "ju2", 50)
		// 显式 INNER 关键字语义一致（带参 ON 用例再复核一次 INNER 前缀）
	})

	t.Run("LEFT JOIN 无匹配补 NULL", func(t *testing.T) {
		drv := seedSuiteJoinUsers(t, f)
		q := drv.Query()

		chain := q.Table("suite_users").
			Select("suite_users.id AS uid", "suite_orders.amount").
			Joins("LEFT JOIN suite_orders ON suite_orders.suite_user_id = suite_users.id").
			Order("suite_users.id")
		left := joinAmountsByUser(t, chain)
		assertAmounts(t, left, "ju1", 100, 250)
		assertAmounts(t, left, "ju2", 50)
		// ju3 行必须存在且 amount 为 NULL/零值形态（驱动归一差异宽松断言：
		// gorm→nil / xorm→零值形态，见 suite_misc ScanMap NULL 差异固化注释）
		list := left["ju3"]
		if len(list) != 1 {
			t.Fatalf("LEFT JOIN 应为无订单用户 ju3 补 1 行，实际 %v", left)
		}

		// u3 行的原始形态直接校验（nil 或零值皆可）
		var rows []map[string]any
		if err := q.Table("suite_users").
			Select("suite_users.id AS uid", "suite_orders.amount").
			Joins("LEFT JOIN suite_orders ON suite_orders.suite_user_id = suite_users.id").
			Where("suite_users.id = ?", "ju3").
			ScanMap(&rows); err != nil {
			t.Fatalf("LEFT JOIN 定向查询: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("LEFT JOIN 期望 ju3 恰 1 行，实际 %d", len(rows))
		}
		if v := rows[0]["amount"]; v != nil {
			switch n := v.(type) {
			case string:
				if n != "" {
					t.Fatalf("ju3 无匹配订单 amount 应为 NULL/零值形态，实际 %#v", v)
				}
			case []byte:
				if len(n) != 0 {
					t.Fatalf("ju3 无匹配订单 amount 应为 NULL/零值形态，实际 %#v", v)
				}
			default:
				if suiteMapNum(t, "amount", v) != 0 {
					t.Fatalf("ju3 无匹配订单 amount 应为 NULL/零值形态，实际 %#v", v)
				}
			}
		}
	})

	t.Run("带参 ON 占位符", func(t *testing.T) {
		drv := seedSuiteJoinUsers(t, f)
		q := drv.Query()

		// ON 条件携带 ? 占位符，变参按序填充（amount >= 100 → 仅 ju1 两单）
		got := joinAmountsByUser(t, q.Table("suite_users").
			Select("suite_users.id AS uid", "suite_orders.amount").
			Joins("INNER JOIN suite_orders ON suite_orders.suite_user_id = suite_users.id "+
				"AND suite_orders.amount >= ?", 100).
			Order("suite_users.id"))
		assertAmounts(t, got, "ju1", 100, 250)
		if _, ok := got["ju2"]; ok {
			t.Fatalf("amount >= 100 不应命中 ju2 的 50，实际 %v", got)
		}
	})

	t.Run("自连接别名查上级", func(t *testing.T) {
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteNode{})
		q := drv.Query()
		mustCreate(t, q,
			&SuiteNode{ID: "sn-root", Name: "root"},
			&SuiteNode{ID: "sn-c1", Name: "child1", ParentID: "sn-root"},
			&SuiteNode{ID: "sn-c2", Name: "child2", ParentID: "sn-root"},
		)

		// 同表 JOIN 别名：子行连出上级行（恰 2 个子节点，parent_name 均 root）
		var rows []map[string]any
		if err := q.Model(&SuiteNode{}).
			Select("suite_nodes.id AS nid", "parent.name AS parent_name").
			Joins("JOIN suite_nodes AS parent ON parent.id = suite_nodes.parent_id").
			Order("suite_nodes.id").
			ScanMap(&rows); err != nil {
			t.Fatalf("自连接 ScanMap: %v", err)
		}
		if len(rows) != 2 {
			t.Fatalf("自连接期望 2 个子节点行，实际 %d: %v", len(rows), rows)
		}
		want := []struct{ nid, pname string }{{"sn-c1", "root"}, {"sn-c2", "root"}}
		for i, w := range want {
			if suiteMapStr(rows[i]["nid"]) != w.nid || suiteMapStr(rows[i]["parent_name"]) != w.pname {
				t.Fatalf("第 %d 行期望 {%s %s}，实际 %v", i, w.nid, w.pname, rows[i])
			}
		}
	})

	t.Run("非法 JOIN 串报错", func(t *testing.T) {
		drv := seedSuiteJoinUsers(t, f)
		q := drv.Query()

		// 非法前缀关键字：只断言「报错」，不锁哨兵（差异固化：gorm 原样透传
		// 由数据库报语法错误 / xorm 解析失败报 ErrUnsupported，见两驱动 fullcov
		// ext Joins 差异断言）
		var rows []map[string]any
		err := q.Table("suite_users").
			Joins("BOGUS JOIN suite_orders ON suite_orders.suite_user_id = suite_users.id").
			ScanMap(&rows)
		if err == nil {
			t.Fatal("非法 JOIN 串（BOGUS 前缀）应报错")
		}
		// 缺 ON 条件不纳入公共断言：SQLite 语法允许 JOIN 不带约束（退化为
		// 笛卡尔积，gorm 侧不报错），仅 xorm 的 gorm 风格解析拒绝——属驱动侧
		// 行为差异，已由两驱动 fullcov ext Joins 差异断言固化。
	})
}
