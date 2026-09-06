// suite_misc.go 代表性用例（orm-tag-design.md）：
//   - suiteAdvanced：§11.6 高级查询矩阵核心行；
//   - suiteExpr：§11.7 表达式更新矩阵核心行；
//   - suiteTx：§11.8 事务矩阵核心行；
//   - suiteLock：§11.10 悲观锁（SQLite 不可跑，整组 Skip + TODO）；
//   - suiteCache：§11.11 查询缓存矩阵代表性用例；
//   - suiteHooks：§11.13 钩子矩阵核心行；
//   - suiteOrmTag：§11.16 ext 标签差异固化断言（SQLite 可跑部分）。
// §11.12 多租户 Schema（SQLite ATTACH 需单连接池开关，工厂未暴露）整组 Skip。

package drivertest

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// ensureLegacyColumn 为 SuiteExt 补建 migration:false 列（gorm 侧 AutoMigrate 不
// 建该列，读写保留——由外部 DDL 建列是 §11.16 规定形态；xorm 侧已由 Sync2 建列，
// 重复 ALTER 报错忽略）。实现取"简单者"：一条幂等 ALTER（失败即已存在）。
func ensureLegacyColumn(t *testing.T, q contracts.Query) {
	t.Helper()
	_ = q.Exec("ALTER TABLE suite_exts ADD COLUMN legacy_code varchar(32) NULL")
}

func seedForMisc(t *testing.T, f Factory) contracts.Driver {
	t.Helper()
	drv := f(t)
	mustAutoMigrate(t, drv, &SuiteUser{}, &SuiteQuota{}, &SuiteVersioned{}, &SuiteSoftDel{}, &SuiteDept{})
	q := drv.Query()
	mustExec(t, q, `INSERT INTO suite_depts (id, name) VALUES ('d1', '研发')`)
	mustCreate(t, q,
		&SuiteUser{ID: "u1", Name: "alice", Email: "alice@test.dev", Age: 30, DeptID: "d1"},
		&SuiteUser{ID: "u2", Name: "bob", Email: "bob@test.dev", Age: 40, DeptID: "d1"},
		&SuiteUser{ID: "u3", Name: "carol", Email: "carol@test.dev", Age: 20, DeptID: "d2"},
		&SuiteQuota{ID: "quota1", Name: "disk", Used: 100},
	)
	return drv
}

// suiteAdvanced §11.6 高级查询矩阵核心行。
func suiteAdvanced(t *testing.T, f Factory) {
	t.Run("FirstOrCreate 未命中创建（ID 自动生成）", func(t *testing.T) {
		drv := seedForMisc(t, f)
		u := &SuiteUser{Name: "foc-new", Email: "foc-new@test.dev"}
		if err := drv.Query().Model(&SuiteUser{}).Where("name = ?", "foc-new").FirstOrCreate(u); err != nil {
			t.Fatalf("FirstOrCreate(未命中): %v", err)
		}
		if len(u.ID) != 16 {
			t.Fatalf("创建分支应生成 16 位 ID，实际 %q", u.ID)
		}
		var count int64
		if err := drv.Query().Model(&SuiteUser{}).Where("name = ?", "foc-new").Count(&count); err != nil || count != 1 {
			t.Fatalf("创建分支应落库 1 行，实际 %d (err=%v)", count, err)
		}
	})

	t.Run("FirstOrCreate 命中返回已有行", func(t *testing.T) {
		drv := seedForMisc(t, f)
		u := &SuiteUser{Name: "alice"}
		if err := drv.Query().Model(&SuiteUser{}).Where("name = ?", "alice").FirstOrCreate(u); err != nil {
			t.Fatalf("FirstOrCreate(命中): %v", err)
		}
		if u.ID != "u1" {
			t.Fatalf("命中应返回已有行 u1，实际 %+v", u)
		}
		var count int64
		if err := drv.Query().Model(&SuiteUser{}).Where("name = ?", "alice").Count(&count); err != nil || count != 1 {
			t.Fatalf("命中分支不应产生新行，实际 %d (err=%v)", count, err)
		}
		// TODO(11.6)：FirstOrCreate 命中分支的 AfterFind 触发与 dest 预填行为
		// 为驱动差异项（gorm 经 gorm 内部路径、xorm 显式回调），差异固化在驱动侧。
	})

	t.Run("FirstOrInit 不落库", func(t *testing.T) {
		drv := seedForMisc(t, f)
		q := drv.Query()
		u := &SuiteUser{Name: "alice"}
		if err := q.Model(&SuiteUser{}).Where("name = ?", "alice").FirstOrInit(u); err != nil {
			t.Fatalf("FirstOrInit(命中): %v", err)
		}
		if u.ID != "u1" {
			t.Fatalf("命中应回填已有行 u1，实际 %+v", u)
		}
		miss := &SuiteUser{Name: "nobody", Email: "nobody@test.dev"}
		if err := q.Model(&SuiteUser{}).Where("name = ?", "nobody").FirstOrInit(miss); err != nil {
			t.Fatalf("FirstOrInit(未命中): %v", err)
		}
		var count int64
		if err := q.Model(&SuiteUser{}).Where("name = ?", "nobody").Count(&count); err != nil || count != 0 {
			t.Fatalf("FirstOrInit 未命中不应落库，实际 %d 行 (err=%v)", count, err)
		}
	})

	t.Run("FindInBatches 尾批与空表", func(t *testing.T) {
		drv := seedForMisc(t, f)
		q := drv.Query()
		isXorm := strings.Contains(drv.DriverName(), "xorm")
		batches := 0
		var all []SuiteUser
		err := q.Model(&SuiteUser{}).Order("id").FindInBatches(&all, 2, func(tx contracts.Query, batch int) error {
			batches++
			if batch != batches {
				t.Errorf("批次序号期望 %d，实际 %d", batches, batch)
			}
			// 逐批内容断言（每批内容有序完整，§11.6）：批 1 = [u1 u2]，批 2 = [u3]。
			// 差异固化（本轮发现）：dest 终态语义不一致——gorm 每批覆盖 dest
			// （fc 内 dest=本批），xorm 追加累计（fc 内 dest=全量前缀窗口）；
			// contracts 契约未定义终态，按 DriverName 取本批窗口断言。
			var got []string
			if isXorm {
				for _, u := range all[(batches-1)*2:] {
					got = append(got, u.ID)
				}
			} else {
				for _, u := range all {
					got = append(got, u.ID)
				}
			}
			want := map[int][]string{1: {"u1", "u2"}, 2: {"u3"}}[batches]
			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Errorf("批 %d 内容期望 %v，实际 %v", batches, want, got)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("FindInBatches: %v", err)
		}
		if batches != 2 {
			t.Fatalf("3 行按 2 分批：期望 2 批，实际 %d 批", batches)
		}
		// 空表：零批回调
		var empty []SuiteSoftDel
		emptyBatches := 0
		if err := q.Model(&SuiteSoftDel{}).FindInBatches(&empty, 2, func(tx contracts.Query, batch int) error {
			emptyBatches++
			return nil
		}); err != nil || emptyBatches != 0 {
			t.Fatalf("空表 FindInBatches 期望 0 批，实际 %d (err=%v)", emptyBatches, err)
		}
		// 非法 batchSize：不 panic（行为差异文档化：gorm 报错 / xorm 0 批）
		var dummy []SuiteUser
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("非法 batchSize 不应 panic: %v", r)
				}
			}()
			_ = q.Model(&SuiteUser{}).FindInBatches(&dummy, 0, func(tx contracts.Query, batch int) error { return nil })
		}()
	})

	t.Run("Scan 标量", func(t *testing.T) {
		drv := seedForMisc(t, f)
		q := drv.Query()
		var n int64
		if err := q.Raw("SELECT count(*) FROM suite_users").Scan(&n); err != nil || n != 3 {
			t.Fatalf("Scan 标量 int64 期望 3，实际 %d (err=%v)", n, err)
		}
		var s string
		if err := q.Raw("SELECT count(*) FROM suite_users").Scan(&s); err != nil {
			t.Fatalf("Scan 标量 string（parseNumeric 兜底）: %q (err=%v)", s, err)
		}
		if n2, perr := strconv.Atoi(strings.TrimSpace(s)); perr != nil || n2 != 3 {
			t.Fatalf("标量字符串应可解析为 3，实际 %q (err=%v)", s, perr)
		}
	})

	t.Run("ScanMap 归一与 NULL 表示", func(t *testing.T) {
		drv := seedForMisc(t, f)
		var rows []map[string]any
		if err := drv.Query().Raw("SELECT id, name, token FROM suite_users ORDER BY id").ScanMap(&rows); err != nil {
			t.Fatalf("ScanMap: %v", err)
		}
		if len(rows) != 3 {
			t.Fatalf("期望 3 行，实际 %d", len(rows))
		}
		// []byte → string 归一（SQLite 文本列驱动可能返回 []byte）
		idv, ok := rows[0]["id"]
		if !ok {
			for k := range rows[0] {
				if strings.EqualFold(k, "id") {
					idv = rows[0][k]
				}
			}
		}
		if _, isStr := idv.(string); !isStr {
			if _, isBytes := idv.([]byte); isBytes {
				t.Fatalf("ScanMap 文本列应归一为 string，实际 []byte")
			}
		}
		// NULL 表示：gorm → nil；xorm QueryInterface 按方言类型扫描 NULL 归一为
		// ""（差异固化，TODO(11.6)：xorm ScanMap NULL 保真待上游支持后对齐）
		isXorm := strings.Contains(drv.DriverName(), "xorm")
		for _, r := range rows {
			var tok any
			found := false
			for k, v := range r {
				if strings.EqualFold(k, "token") {
					tok, found = v, true
				}
			}
			if !found {
				continue
			}
			if isXorm {
				if s, ok := tok.(string); !ok || s != "" {
					t.Fatalf("xorm 侧 NULL 应归一为空串（差异固化），实际 %#v", tok)
				}
			} else if tok != nil {
				t.Fatalf("gorm 侧 NULL 列应表示为 nil，实际 %#v", tok)
			}
		}
		// 追加语义：二次 ScanMap 追加而非替换
		if err := drv.Query().Raw("SELECT id FROM suite_users LIMIT 1").ScanMap(&rows); err != nil {
			t.Fatalf("ScanMap 追加: %v", err)
		}
		if len(rows) != 4 {
			t.Fatalf("ScanMap 应追加（期望 4 行），实际 %d", len(rows))
		}
	})

	t.Run("Raw + Row/Rows（原生路径）", func(t *testing.T) {
		drv := seedForMisc(t, f)
		q := drv.Query()
		row := q.Raw("SELECT count(*) FROM suite_users").Row()
		var n int64
		if err := row.Scan(&n); err != nil || n != 3 {
			t.Fatalf("Row().Scan 期望 3，实际 %d (err=%v)", n, err)
		}
		rows, err := q.Raw("SELECT id FROM suite_users ORDER BY id").Rows()
		if err != nil {
			t.Fatalf("Rows(): %v", err)
		}
		defer rows.Close()
		count := 0
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				t.Fatalf("Rows 迭代 Scan: %v", err)
			}
			count++
		}
		if count != 3 {
			t.Fatalf("Rows 迭代期望 3 行，实际 %d", count)
		}
	})
}

// suiteExpr §11.7 表达式更新矩阵核心行。
func suiteExpr(t *testing.T, f Factory) {
	t.Run("Expr 原子自增与混排", func(t *testing.T) {
		drv := seedForMisc(t, f)
		q := drv.Query()

		// 原子自增：100 次累加 = 初值 + 100（数据库端执行）
		for i := 0; i < 100; i++ {
			if err := q.Model(&SuiteQuota{}).Where("id = ?", "quota1").Update("used", contracts.Expr("used + ?", 1)); err != nil {
				t.Fatalf("Expr 自增: %v", err)
			}
		}
		var quota SuiteQuota
		mustTake(t, q, &quota, "quota1")
		if quota.Used != 200 {
			t.Fatalf("100 次原子自增后期望 200，实际 %d", quota.Used)
		}

		// 表达式与普通值混排
		if err := q.Model(&SuiteQuota{}).Where("id = ?", "quota1").
			Updates(map[string]any{"used": contracts.Expr("used - ?", 50), "name": "disk2"}); err != nil {
			t.Fatalf("Expr 混排: %v", err)
		}
		mustTake(t, q, &quota, "quota1")
		if quota.Used != 150 || quota.Name != "disk2" {
			t.Fatalf("混排更新不一致: %+v", quota)
		}

		// 带条件的表达式更新仅命中行
		if err := q.Model(&SuiteQuota{}).Where("name = ?", "no-such").Update("used", contracts.Expr("used + ?", 999)); err != nil {
			t.Fatalf("Expr 条件更新: %v", err)
		}
		mustTake(t, q, &quota, "quota1")
		if quota.Used != 150 {
			t.Fatalf("未命中行不应被更新，实际 %d", quota.Used)
		}

		// UpdatesResult 带表达式：行数回填（SQLite 用 MAX 作标量最大函数；
		// GREATEST 为 MySQL/PG 方言——方言函数下推由 §11.7/§11.18 集成测试覆盖）
		r := q.Model(&SuiteQuota{}).Where("id = ?", "quota1").UpdatesResult(map[string]any{
			"used": contracts.Expr("MAX(used - ?, 0)", 100),
		})
		if r.Error != nil || r.RowsAffected != 1 {
			t.Fatalf("UpdatesResult(Expr) 期望 1 行，实际 %+v", r)
		}
		mustTake(t, q, &quota, "quota1")
		if quota.Used != 50 {
			t.Fatalf("GREATEST 下推后期望 50，实际 %d", quota.Used)
		}
	})
}

// suiteTx §11.8 事务矩阵核心行。
func suiteTx(t *testing.T, f Factory) {
	t.Run("Transaction 提交与回滚", func(t *testing.T) {
		drv := seedForMisc(t, f)
		q := drv.Query()

		err := q.Transaction(func(tx contracts.Query) error {
			return tx.Create(&SuiteUser{ID: "txok00000000001", Name: "in-tx", Email: "in-tx@test.dev"})
		})
		if err != nil {
			t.Fatalf("Transaction 提交: %v", err)
		}
		var count int64
		if err := q.Model(&SuiteUser{}).Where("id = ?", "txok00000000001").Count(&count); err != nil || count != 1 {
			t.Fatalf("事务提交应落库，实际 %d (err=%v)", count, err)
		}

		rollbackErr := errors.New("rollback-please")
		err = q.Transaction(func(tx contracts.Query) error {
			if err := tx.Create(&SuiteUser{ID: "txrb00000000001", Name: "will-rb", Email: "rb@test.dev"}); err != nil {
				return err
			}
			return rollbackErr
		})
		if err == nil {
			t.Fatal("fc 错误应透传")
		}
		if err := q.Model(&SuiteUser{}).Where("id = ?", "txrb00000000001").Count(&count); err != nil || count != 0 {
			t.Fatalf("回滚后不应落库，实际 %d 行 (err=%v)", count, err)
		}
	})

	t.Run("Begin/Commit 与 Begin/Rollback", func(t *testing.T) {
		drv := seedForMisc(t, f)
		q := drv.Query()

		tx := q.Begin()
		mustCreate(t, tx, &SuiteUser{ID: "manual0000000001", Name: "manual", Email: "m@test.dev"})
		if err := tx.Commit(); err != nil {
			t.Fatalf("手动 Commit: %v", err)
		}
		var count int64
		if err := q.Model(&SuiteUser{}).Where("id = ?", "manual0000000001").Count(&count); err != nil || count != 1 {
			t.Fatalf("手动提交应落库，实际 %d (err=%v)", count, err)
		}

		tx2 := q.Begin()
		mustCreate(t, tx2, &SuiteUser{ID: "manualrb00000001", Name: "manual-rb", Email: "mrb@test.dev"})
		if err := tx2.Rollback(); err != nil {
			t.Fatalf("手动 Rollback: %v", err)
		}
		if err := q.Model(&SuiteUser{}).Where("id = ?", "manualrb00000001").Count(&count); err != nil || count != 0 {
			t.Fatalf("手动回滚后不应落库，实际 %d 行 (err=%v)", count, err)
		}
	})

	t.Run("SavePoint 部分回滚", func(t *testing.T) {
		drv := seedForMisc(t, f)
		tx := drv.Query().Begin()
		defer func() { _ = tx.Rollback() }()

		mustCreate(t, tx, &SuiteUser{ID: "sp-keep00000001", Name: "keep", Email: "keep@test.dev"})
		if err := tx.SavePoint("sp1"); err != nil {
			t.Fatalf("SavePoint: %v", err)
		}
		mustCreate(t, tx, &SuiteUser{ID: "sp-drop00000001", Name: "drop", Email: "drop@test.dev"})
		if err := tx.RollbackTo("sp1"); err != nil {
			t.Fatalf("RollbackTo: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit: %v", err)
		}

		q := drv.Query()
		var count int64
		if err := q.Model(&SuiteUser{}).Where("id = ?", "sp-keep00000001").Count(&count); err != nil || count != 1 {
			t.Fatalf("保存点之前的写入应保留，实际 %d (err=%v)", count, err)
		}
		if err := q.Model(&SuiteUser{}).Where("id = ?", "sp-drop00000001").Count(&count); err != nil || count != 0 {
			t.Fatalf("保存点之后的写入应回滚，实际 %d 行 (err=%v)", count, err)
		}
	})

	t.Run("TxOption 双驱动均不报错（差异固化）", func(t *testing.T) {
		drv := seedForMisc(t, f)
		// gorm 透传隔离级别；xorm 降级忽略（§11.8：差异固化断言：均不报错）
		err := drv.Query().Transaction(func(tx contracts.Query) error {
			return tx.Create(&SuiteUser{ID: "txopt0000000001", Name: "opt", Email: "opt@test.dev"})
		}, contracts.TxReadCommitted)
		if err != nil {
			t.Fatalf("TxOption 事务不应报错（差异固化）: %v", err)
		}
	})

	// TODO(11.8)：fc panic 回滚路径（gorm 原生回滚重抛 / xorm Close 兜底——行为
	// 差异文档化）；嵌套 Transaction（gorm savepoint 语义 / xorm 独立会话语义，
	// SQLite 单连接下有锁死风险）；事务内 Cache 隔离性。

	t.Run("事务内读写一致", func(t *testing.T) {
		drv := seedForMisc(t, f)
		err := drv.Query().Transaction(func(tx contracts.Query) error {
			var u SuiteUser
			if err := tx.Model(&SuiteUser{}).Where("id = ?", "u1").Take(&u); err != nil {
				return err
			}
			u.Age = 99
			return tx.Model(&SuiteUser{}).Where("id = ?", "u1").Update("age", 99)
		})
		if err != nil {
			t.Fatalf("事务内读写: %v", err)
		}
		var u SuiteUser
		mustTake(t, drv.Query(), &u, "u1")
		if u.Age != 99 {
			t.Fatalf("事务内更新未生效: %+v", u)
		}
	})
}

// suiteLock §11.10 悲观锁矩阵：SQLite 不支持 FOR UPDATE/SHARE 语法，锁 SQL 生成
// 断言与并发互斥行为需 PG/MySQL 集成测试（§11.18，GOFAST_TEST_PG_DSN 等）。
func suiteLock(t *testing.T, f Factory) {
	t.Skip("TODO(11.10)：FOR UPDATE/SHARE 生成与并发互斥需 PG/MySQL 集成测试" +
		"（orm-tag-design.md §11.10/§11.18）；SQLite 不支持锁语法，锁+缓存剥离行为" +
		"已由驱动侧单测覆盖")
}

// suiteCache §11.11 查询缓存矩阵代表性用例（命中/写失效/TTL/Tags/dest 隔离）。
// 缓存命中以 QueryCounter 的 SQL 计数观测（未实现计数器则整组 t.Skip）。
func suiteCache(t *testing.T, f Factory) {
	drv := seedForMisc(t, f)
	count, ok := queryCountOf(drv)
	if !ok {
		t.Skip("TODO(11.11)：驱动未实现 QueryCounter，无法无副作用地观测缓存命中；" +
			"接入计数器后启用本组（§11.11）")
		return
	}
	enable, ok := drv.(contracts.QueryCacher)
	if !ok {
		t.Skip("TODO(11.11)：驱动未实现 contracts.QueryCacher")
		return
	}
	sc := newSuiteCache()
	if err := enable.EnableCaches(sc); err != nil {
		t.Fatalf("EnableCaches: %v", err)
	}
	q := drv.Query()

	t.Run("命中与未命中", func(t *testing.T) {
		var rows []SuiteUser
		before := count()
		if err := q.Cache().Model(&SuiteUser{}).Where("dept_id = ?", "d1").Order("id").Find(&rows); err != nil {
			t.Fatalf("首次缓存查询: %v", err)
		}
		if n := count() - before; n == 0 {
			t.Fatal("首次查询应回源 DB")
		}
		if len(rows) != 2 {
			t.Fatalf("d1 期望 2 行，实际 %d", len(rows))
		}
		// 二次同链查询：命中缓存，零 DB 访问
		var again []SuiteUser
		before = count()
		if err := q.Cache().Model(&SuiteUser{}).Where("dept_id = ?", "d1").Order("id").Find(&again); err != nil {
			t.Fatalf("缓存命中查询: %v", err)
		}
		if n := count() - before; n != 0 {
			t.Fatalf("命中缓存不应访问 DB，实际 %d 次", n)
		}
		if len(again) != 2 || again[0].ID != rows[0].ID {
			t.Fatalf("缓存结果应一致: %+v", again)
		}
		// 未调 Cache() 的查询零副作用：数据变化后立即反映
		mustExec(t, q, "UPDATE suite_users SET age = 77 WHERE id = ?", "u1")
		var fresh []SuiteUser
		if err := q.Model(&SuiteUser{}).Where("dept_id = ?", "d1").Order("id").Find(&fresh); err != nil {
			t.Fatalf("无缓存查询: %v", err)
		}
		if fresh[0].Age != 77 {
			t.Fatalf("未开缓存查询应回源，实际 %+v", fresh[0])
		}
	})

	t.Run("写操作失效", func(t *testing.T) {
		var rows []SuiteUser
		if err := q.Cache().Model(&SuiteUser{}).Where("dept_id = ?", "d2").Find(&rows); err != nil {
			t.Fatalf("缓存查询: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("d2 期望 1 行，实际 %d", len(rows))
		}
		// Create 写失效：新行立即可见（且触发回源）
		mustCreate(t, q, &SuiteUser{ID: "cache0000000001", Name: "dave", Email: "dave@test.dev", DeptID: "d2"})
		var again []SuiteUser
		if err := q.Cache().Model(&SuiteUser{}).Where("dept_id = ?", "d2").Find(&again); err != nil {
			t.Fatalf("写后缓存查询: %v", err)
		}
		if len(again) != 2 {
			t.Fatalf("写操作应失效缓存（期望 2 行），实际 %d", len(again))
		}
	})

	t.Run("TTL 过期回源", func(t *testing.T) {
		var rows []SuiteUser
		before := count()
		if err := q.Cache(contracts.CacheTTLOption(60*time.Millisecond)).
			Model(&SuiteUser{}).Where("name = ?", "alice").Find(&rows); err != nil {
			t.Fatalf("TTL 查询: %v", err)
		}
		if n := count() - before; n == 0 {
			t.Fatal("TTL 查询首次应回源")
		}
		var cached []SuiteUser
		before = count()
		if err := q.Cache(contracts.CacheTTLOption(60*time.Millisecond)).
			Model(&SuiteUser{}).Where("name = ?", "alice").Find(&cached); err != nil {
			t.Fatalf("TTL 命中查询: %v", err)
		}
		if n := count() - before; n != 0 {
			t.Fatalf("TTL 内应命中缓存，实际 %d 次 DB 访问", n)
		}
		time.Sleep(150 * time.Millisecond) // 过期
		before = count()
		var expired []SuiteUser
		if err := q.Cache(contracts.CacheTTLOption(60*time.Millisecond)).
			Model(&SuiteUser{}).Where("name = ?", "alice").Find(&expired); err != nil {
			t.Fatalf("TTL 过期查询: %v", err)
		}
		if n := count() - before; n == 0 {
			t.Fatal("TTL 过期后应回源 DB")
		}
	})

	t.Run("Tags 手动批量失效", func(t *testing.T) {
		var rows []SuiteUser
		if err := q.Cache(contracts.CacheTagsOption("suite-users")).
			Model(&SuiteUser{}).Where("name = ?", "bob").Find(&rows); err != nil {
			t.Fatalf("Tags 查询: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("bob 期望 1 行，实际 %d", len(rows))
		}
		// 标签 Flush 后回源（驱动缓存承载于同一 suiteCache 实例 sc）
		if err := sc.Tags("suite-users").Flush(); err != nil {
			t.Fatalf("Tags Flush: %v", err)
		}
		mustExec(t, q, "UPDATE suite_users SET age = 66 WHERE name = ?", "bob")
		var fresh []SuiteUser
		if err := q.Cache(contracts.CacheTagsOption("suite-users")).
			Model(&SuiteUser{}).Where("name = ?", "bob").Find(&fresh); err != nil {
			t.Fatalf("Tags Flush 后查询: %v", err)
		}
		if fresh[0].Age != 66 {
			t.Fatalf("Tags Flush 后应回源读到新值，实际 %+v", fresh[0])
		}
	})

	t.Run("软删标记路径写失效", func(t *testing.T) {
		mustAutoMigrate(t, drv, &SuiteSoftDel{})
		mustCreate(t, q, &SuiteSoftDel{ID: "cache-sd-00001", Name: "cachable"})
		var rows []SuiteSoftDel
		if err := q.Cache().Model(&SuiteSoftDel{}).Where("deleted_at = ?", 0).Find(&rows); err != nil {
			t.Fatalf("缓存查询: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("期望 1 行存活，实际 %d", len(rows))
		}
		// 业务级软删标记经驱动写路径 Update → 缓存失效
		if err := q.Model(&SuiteSoftDel{}).Where("id = ?", "cache-sd-00001").Update("deleted_at", time.Now().Unix()); err != nil {
			t.Fatalf("软删标记: %v", err)
		}
		var again []SuiteSoftDel
		if err := q.Cache().Model(&SuiteSoftDel{}).Where("deleted_at = ?", 0).Find(&again); err != nil {
			t.Fatalf("软删后缓存查询: %v", err)
		}
		if len(again) != 0 {
			t.Fatalf("软删标记应失效缓存（期望 0 行），实际 %d", len(again))
		}
	})

	// TODO(11.11)：多 Store 隔离 / Row/Rows/FindInBatches 不缓存 / 不可序列化
	// dest 静默跳过 / Preload 子查询继承缓存标记——见文档 §11.11 对应行。
}

// suiteHooks §11.13 钩子矩阵核心行。
func suiteHooks(t *testing.T, f Factory) {
	t.Run("Create 两钩子与切片逐元素", func(t *testing.T) {
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteHook{})
		q := drv.Query()

		h := &SuiteHook{Name: "single"}
		mustCreate(t, q, h)
		if !h.BeforeCreateCalled || !h.AfterCreateCalled {
			t.Fatalf("Create 应触发 Before/AfterCreate: %+v", h)
		}
		// CreateInBatches：每元素各一次
		hs := []*SuiteHook{{Name: "b1"}, {Name: "b2"}, {Name: "b3"}}
		if err := q.CreateInBatches(hs, 2); err != nil {
			t.Fatalf("CreateInBatches: %v", err)
		}
		for i, h := range hs {
			if !h.BeforeCreateCalled || !h.AfterCreateCalled {
				t.Fatalf("第 %d 元素钩子未触发: %+v", i, h)
			}
		}
	})

	t.Run("Update/Updates 不触发模型钩子", func(t *testing.T) {
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteHook{})
		q := drv.Query()
		h := &SuiteHook{Name: "up"}
		mustCreate(t, q, h)

		// 链式 Update/Updates：不触发任何模型钩子（§11.13 文档化行为）
		if err := q.Model(&SuiteHook{}).Where("id = ?", h.ID).Update("name", "up2"); err != nil {
			t.Fatalf("Update: %v", err)
		}
		if err := q.Model(&SuiteHook{}).Where("id = ?", h.ID).Updates(map[string]any{"name": "up3"}); err != nil {
			t.Fatalf("Updates: %v", err)
		}
		var got SuiteHook
		mustTake(t, q, &got, h.ID)
		if got.BeforeUpdateCalled || got.AfterUpdateCalled || got.BeforeCreateCalled {
			t.Fatalf("Update/Updates 不应触发模型钩子: %+v", got)
		}

		// Save 更新分支：触发 Update 系钩子
		got.Name = "save-updated"
		if err := q.Save(&got); err != nil {
			t.Fatalf("Save(更新): %v", err)
		}
		if !got.BeforeUpdateCalled || !got.AfterUpdateCalled {
			t.Fatalf("Save 更新分支应触发 Update 系钩子: %+v", got)
		}
	})

	t.Run("AfterFind 触发时机", func(t *testing.T) {
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteHook{})
		q := drv.Query()
		h := &SuiteHook{Name: "find"}
		mustCreate(t, q, h)

		var got SuiteHook
		if err := q.Model(&SuiteHook{}).Where("id = ?", h.ID).First(&got); err != nil {
			t.Fatalf("First: %v", err)
		}
		if !got.AfterFindCalled {
			t.Fatalf("查到数据应触发 AfterFind: %+v", got)
		}
		// 空结果不触发
		var none SuiteHook
		_ = q.Model(&SuiteHook{}).Where("id = ?", "ghost").First(&none)
		if none.AfterFindCalled {
			t.Fatal("空结果不应触发 AfterFind")
		}
	})

	t.Run("Delete 系钩子", func(t *testing.T) {
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteHook{})
		q := drv.Query()
		h := &SuiteHook{Name: "del"}
		mustCreate(t, q, h)

		if err := q.Delete(&SuiteHook{}, "id = ?", h.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		// Delete 的 Before/AfterDelete 作用于传入 value——conds 形态下 value 为
		// 模型类型载体（零值），钩子标记在传入值上断言
		h2 := &SuiteHook{Name: "del2"}
		mustCreate(t, q, h2)
		if err := q.Delete(h2); err != nil {
			t.Fatalf("Delete(value): %v", err)
		}
		if !h2.BeforeDeleteCalled || !h2.AfterDeleteCalled {
			t.Fatalf("Delete 应触发 Before/AfterDelete: %+v", h2)
		}
	})

	t.Run("钩子错误中断与透传", func(t *testing.T) {
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteHook{})
		q := drv.Query()

		h := &SuiteHook{Name: "boom", FailHook: "before_create"}
		if err := q.Create(h); err == nil {
			t.Fatal("BeforeCreate 错误应中断 Create")
		}
		var count int64
		if err := q.Model(&SuiteHook{}).Where("name = ?", "boom").Count(&count); err != nil || count != 0 {
			t.Fatalf("错误中断应不落库，实际 %d 行 (err=%v)", count, err)
		}
	})
}

// suiteOrmTag §11.16 ext 标签差异固化（SQLite 可跑部分）。
func suiteOrmTag(t *testing.T, f Factory) {
	drv := f(t)
	mustAutoMigrate(t, drv, &SuiteExt{})
	q := drv.Query()
	ensureLegacyColumn(t, q)
	isXorm := strings.Contains(drv.DriverName(), "xorm")

	t.Run("migration:false 列读写（补列后）", func(t *testing.T) {
		e := &SuiteExt{Title: "ext1", Age: 20, OrgID: "o1", DevID: "d1", LegacyCode: "LEG-1"}
		mustCreate(t, q, e)
		var got SuiteExt
		mustTake(t, q, &got, e.ID)
		if got.LegacyCode != "LEG-1" {
			t.Fatalf("migration:false 列应读写正常，实际 %+v", got)
		}
	})

	t.Run("check 约束（差异固化）", func(t *testing.T) {
		bad := &SuiteExt{Title: "bad-age", Age: -1, OrgID: "o2", DevID: "d2"}
		err := q.Create(bad)
		if isXorm {
			// xorm 忽略 ext check（差异固化）：违反值写入成功
			if err != nil {
				t.Fatalf("xorm 侧无 CHECK，违反值应写入成功（差异固化）: %v", err)
			}
		} else if err == nil {
			t.Fatal("gorm 侧 CHECK(age >= 0) 应拒绝违反行")
		}
	})

	t.Run("timePrecision milli（差异固化）", func(t *testing.T) {
		e := &SuiteExt{Title: "milli", Age: 1, OrgID: "o3", DevID: "d3"}
		mustCreate(t, q, e)
		var got SuiteExt
		mustTake(t, q, &got, e.ID)
		// gorm 填 unix 毫秒（>1e12）；xorm created 恒为秒（1e9~1e10）——差异固化
		if isXorm {
			if got.MilliCreated < 1_000_000_000 || got.MilliCreated > 10_000_000_000 {
				t.Fatalf("xorm 侧 created 应为 unix 秒（差异固化），实际 %d", got.MilliCreated)
			}
		} else if got.MilliCreated < 1_000_000_000_000 {
			t.Fatalf("gorm 侧 milli 精度应填 unix 毫秒，实际 %d", got.MilliCreated)
		}
	})

	t.Run("perm:create（差异固化）", func(t *testing.T) {
		e := &SuiteExt{Title: "perm", Age: 5, OrgID: "o4", DevID: "d4", BornAt: 111}
		mustCreate(t, q, e)
		var got SuiteExt
		mustTake(t, q, &got, e.ID)
		if got.BornAt != 111 {
			t.Fatalf("perm:create 字段应随 Create 写入，实际 %d", got.BornAt)
		}
		if err := q.Model(&SuiteExt{}).Where("id = ?", e.ID).Updates(map[string]any{"born_at": 222, "title": "perm2"}); err != nil {
			t.Fatalf("Updates: %v", err)
		}
		var after SuiteExt
		mustTake(t, q, &after, e.ID)
		if isXorm {
			// xorm 不拦截写路径（差异固化）：Updates 覆盖 perm:create 字段
			if after.BornAt != 222 {
				t.Fatalf("xorm 侧 perm:create 由业务保证（差异固化），实际 %d", after.BornAt)
			}
		} else if after.BornAt != 111 || after.Title != "perm2" {
			t.Fatalf("gorm 侧 perm:create 字段 Updates 应跳过（born_at 保持 111），实际 %+v", after)
		}
	})

	// TODO(11.16)：未知 ext key 的 Warn 日志断言（依赖捕获型日志器，驱动侧已覆盖）
	// TODO(11.12)：多租户 Schema（SQLite ATTACH 需单连接池开关，工厂未暴露）——
	// 代表性用例见各驱动包内 query_schema_test.go。
}
