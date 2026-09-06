// suite_ddl.go AutoMigrate DDL 等价性测试（orm-tag-design.md §11.15 SQLite 子集）。
//
// 套件按工厂逐驱动运行（同一时刻只有一个驱动的库可读），因此"双驱动等价"
// 通过同一份**规范期望**表达：读 sqlite_master 原始 DDL → 归一化 → 逐项断言
// 列名集合/主键/NOT NULL/默认值/唯一/索引。两驱动对同一期望各自成立，即等价。
// 方言差异（ext 高级选项、migration:false）以 DriverName 分支固化（§11.16）。

package drivertest

import (
	"sort"
	"strings"
	"testing"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// ddlModels DDL 等价性覆盖的模型全集。
var ddlModels = []any{
	&SuiteUser{}, &SuiteSoftDel{}, &SuiteRole{}, &SuitePhoto{},
	&SuiteUserRole{}, &SuiteVersioned{}, &SuiteExt{},
}

// loadDDL 读 sqlite_master（type IN table/index 且 sql 非空），返回：
//   - "table:<表名>" → 归一化 CREATE TABLE 语句；
//   - "index:<索引名>" → 归一化 CREATE INDEX 语句；
//   - "tbl:<表名>" → 该表的建表与全部相关索引 DDL 拼接（contains 断言用）。
func loadDDL(t *testing.T, q contracts.Query) map[string]string {
	t.Helper()
	var rows []map[string]any
	if err := q.Raw("SELECT name, tbl_name, type, sql FROM sqlite_master WHERE type IN ('table','index') AND sql IS NOT NULL").ScanMap(&rows); err != nil {
		t.Fatalf("读取 sqlite_master: %v", err)
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		sql := normalizeDDL(ddlMapStr(r, "sql"))
		name := ddlMapStr(r, "name")
		tbl := ddlMapStr(r, "tbl_name")
		kind := ddlMapStr(r, "type")
		out[kind+":"+name] = sql
		out["tbl:"+tbl] += "\n" + sql
	}
	return out
}

func ddlMapStr(row map[string]any, key string) string {
	if v, ok := row[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	for k, v := range row { // 部分驱动列名大小写漂移兜底
		if strings.EqualFold(k, key) {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

// normalizeDDL 归一化：去反引号/双引号、小写、折叠空白、去 IF NOT EXISTS。
func normalizeDDL(s string) string {
	s = strings.ReplaceAll(s, "`", "")
	s = strings.ReplaceAll(s, `"`, "")
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "if not exists ", "")
	return strings.Join(strings.Fields(s), " ")
}

// ddlColumnDefs 拆出归一化建表 SQL 的顶层（括号深度 0）定义片段。
func ddlColumnDefs(createSQL string) []string {
	open := strings.Index(createSQL, "(")
	closeIdx := strings.LastIndex(createSQL, ")")
	if open < 0 || closeIdx <= open {
		return nil
	}
	body := createSQL[open+1 : closeIdx]
	var defs []string
	depth, start := 0, 0
	for i := 0; i < len(body); i++ {
		switch body[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				if def := strings.TrimSpace(body[start:i]); def != "" {
					defs = append(defs, def)
				}
				start = i + 1
			}
		}
	}
	if rest := strings.TrimSpace(body[start:]); rest != "" {
		defs = append(defs, rest)
	}
	return defs
}

// ddlColumns 提取列名集合（跳过 primary/unique/check/foreign/constraint 表级约束）。
func ddlColumns(createSQL string) []string {
	var cols []string
	for _, def := range ddlColumnDefs(createSQL) {
		first := def
		if i := strings.IndexByte(def, ' '); i >= 0 {
			first = def[:i]
		}
		switch first {
		case "primary", "unique", "check", "foreign", "constraint":
			continue
		}
		cols = append(cols, first)
	}
	return cols
}

// ddlHasColumnAttr 判断 <col> 列定义是否包含 <attr> 片段（attr 为空仅判断列存在）。
func ddlHasColumnAttr(createSQL, col, attr string) bool {
	for _, def := range ddlColumnDefs(createSQL) {
		first := def
		if i := strings.IndexByte(def, ' '); i >= 0 {
			first = def[:i]
		}
		if first != col {
			continue
		}
		return attr == "" || strings.Contains(def, attr)
	}
	return false
}

func assertColumnsEqual(t *testing.T, label string, got, want []string) {
	t.Helper()
	a, b := append([]string(nil), got...), append([]string(nil), want...)
	sort.Strings(a)
	sort.Strings(b)
	if strings.Join(a, ",") != strings.Join(b, ",") {
		t.Fatalf("%s 列集合不等价:\n 期望 %v\n 实际 %v", label, b, a)
	}
}

func suiteDDL(t *testing.T, f Factory) {
	drv := f(t)
	mustAutoMigrate(t, drv, ddlModels...)
	q := drv.Query()
	ddl := loadDDL(t, q)
	isXorm := strings.Contains(drv.DriverName(), "xorm")

	t.Run("列集合与忽略字段", func(t *testing.T) {
		if ddl["table:suite_users"] == "" {
			t.Fatal("suite_users 表应存在")
		}
		assertColumnsEqual(t, "suite_users", ddlColumns(ddl["table:suite_users"]), []string{
			"id", "name", "email", "age", "score", "password", "token", "dept_id", "created_at", "updated_at",
		}) // internal（orm:"-"）必须无列
		assertColumnsEqual(t, "suite_soft_dels", ddlColumns(ddl["table:suite_soft_dels"]),
			[]string{"id", "name", "deleted_at"})
		assertColumnsEqual(t, "suite_photos", ddlColumns(ddl["table:suite_photos"]),
			[]string{"id", "owner_id", "owner_type", "url"})
		assertColumnsEqual(t, "suite_user_roles", ddlColumns(ddl["table:suite_user_roles"]),
			[]string{"id", "code"})
		assertColumnsEqual(t, "suite_versioned", ddlColumns(ddl["table:suite_versioned"]),
			[]string{"id", "title", "version"})
	})

	t.Run("主键/非空/默认值/类型映射", func(t *testing.T) {
		users := ddl["table:suite_users"]
		if !strings.Contains(users, "primary key") {
			t.Fatalf("suite_users 应含主键约束: %s", users)
		}
		if !ddlHasColumnAttr(users, "name", "not null") {
			t.Fatalf("suite_users.name 应 NOT NULL: %s", users)
		}
		if !ddlHasColumnAttr(users, "age", "default") {
			t.Fatalf("suite_users.age 应有 DEFAULT: %s", users)
		}
		// 类型映射等价（§11.15"按引擎类型映射表逐引擎断言"）：SQLite 下 gorm 以
		// orm token 原文落地（patch 透传），xorm 经方言映射折算为亲和类型——
		// varchar(100)→text、decimal(10,2)→numeric，同一亲和性即等价。
		if strings.Contains(drv.DriverName(), "xorm") {
			if !ddlHasColumnAttr(users, "name", "text") {
				t.Fatalf("xorm 侧 suite_users.name 应为 text 亲和: %s", users)
			}
			if !ddlHasColumnAttr(users, "score", "numeric") {
				t.Fatalf("xorm 侧 suite_users.score 应为 numeric 亲和: %s", users)
			}
		} else {
			if !ddlHasColumnAttr(users, "name", "varchar(100)") {
				t.Fatalf("suite_users.name 列型应含 varchar(100): %s", users)
			}
			if !ddlHasColumnAttr(users, "score", "decimal(10,2)") {
				t.Fatalf("suite_users.score 列型应含 decimal(10,2): %s", users)
			}
		}
		if !ddlHasColumnAttr(ddl["table:suite_soft_dels"], "deleted_at", "default") {
			t.Fatalf("suite_soft_dels.deleted_at 应有 DEFAULT 0")
		}
	})

	t.Run("列级唯一与索引", func(t *testing.T) {
		// 列级唯一：gorm 落在建表 DDL 内联；xorm 落在独立 CREATE UNIQUE INDEX
		//（UQE_ 前缀），统一在"表+全部索引"聚合 DDL 上断言
		if !strings.Contains(ddl["tbl:suite_users"], "unique") {
			t.Fatalf("suite_users.email 列级唯一应存在: %s", ddl["tbl:suite_users"])
		}
		soft := ddl["tbl:suite_soft_dels"]
		if !strings.Contains(soft, "index") || !strings.Contains(soft, "deleted_at") {
			t.Fatalf("suite_soft_dels.deleted_at 应有索引: %s", soft)
		}
		// 联合索引组 idx_suite_ext_org_dev（org_id+dev_id 同组，§11.15 联合索引行）
		found := false
		for k, sql := range ddl {
			if strings.HasPrefix(k, "index:") &&
				strings.Contains(sql, "org_id") && strings.Contains(sql, "dev_id") {
				found = true
			}
		}
		if !found {
			t.Fatalf("suite_exts 应存在 (org_id, dev_id) 联合索引: %v", ddl)
		}
	})

	t.Run("extends 前缀与嵌入平铺", func(t *testing.T) {
		want := []string{
			"id", "title", "age", "org_id", "dev_id", "email", "step", "milli_created", "born_at",
			"audit_note", "author_name", "author_bio",
		}
		if isXorm {
			// 差异固化（§11.16）：migration:false 在 xorm 侧照常建列
			want = append(want, "legacy_code")
		}
		assertColumnsEqual(t, "suite_exts", ddlColumns(ddl["table:suite_exts"]), want)
	})

	t.Run("migration:false（差异固化）", func(t *testing.T) {
		ext := ddl["table:suite_exts"]
		if isXorm {
			if !ddlHasColumnAttr(ext, "legacy_code", "") {
				t.Fatalf("xorm 侧 legacy_code 应由 Sync2 建列（差异固化）: %s", ext)
			}
		} else if ddlHasColumnAttr(ext, "legacy_code", "") {
			t.Fatalf("gorm 侧 migration:false 不应建列: %s", ext)
		}
	})

	t.Run("ext 高级选项（差异固化）", func(t *testing.T) {
		ext := ddl["tbl:suite_exts"]
		if isXorm {
			if strings.Contains(ext, "check") {
				t.Fatalf("xorm 侧不应生成 CHECK（差异固化）: %s", ext)
			}
		} else {
			if !ddlHasColumnAttr(ext, "age", "check") && !strings.Contains(ext, "check") {
				t.Fatalf("gorm 侧应生成 CHECK(age >= 0): %s", ext)
			}
			if !strings.Contains(ext, "idx_suite_ext_email") {
				t.Fatalf("gorm 侧应为 email 建 ext 索引 idx_suite_ext_email: %s", ext)
			}
		}
	})

	t.Run("幂等：重复 AutoMigrate 不报错且不重复建索引", func(t *testing.T) {
		countSoftDelIndexes := func(d map[string]string) int {
			n := 0
			for k := range d {
				if strings.HasPrefix(k, "index:") && strings.Contains(k, "suite_soft_dels") {
					n++
				}
			}
			return n
		}
		before := countSoftDelIndexes(ddl)
		if err := drv.AutoMigrate(ddlModels...); err != nil {
			t.Fatalf("重复 AutoMigrate: %v", err)
		}
		if after := countSoftDelIndexes(loadDDL(t, q)); after != before {
			t.Fatalf("重复 AutoMigrate 不应新增索引（before=%d after=%d）", before, after)
		}
	})

	// TODO(11.15)：以下行为本轮未纳入，见文档对应行——
	//   - comment('...')：MySQL/PG 方言，SQLite 跳过；
	//   - unsigned 数值列 / autoIncrementIncrement:5：MySQL 方言；
	//   - 非导出嵌入类型静默跳过（差异固化 + 文档警示）；
	//   - TablePrefix 配置场景（套件工厂未暴露前缀开关）。
}
