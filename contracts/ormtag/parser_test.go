package ormtag

import (
	"reflect"
	"strings"
	"testing"
)

// ── 测试辅助 ─────────────────────────────────────────────────────────────

// parseOneModel 用「单字段模型」解析 orm tag，返回 ModelMeta。
func parseOneModel(t *testing.T, ormTag string) *ModelMeta {
	t.Helper()
	st := reflect.StructOf([]reflect.StructField{{
		Name: "F",
		Type: reflect.TypeOf(""),
		Tag:  reflect.StructTag(`orm:"` + ormTag + `"`),
	}})
	m, err := Parse(reflect.New(st).Interface())
	if err != nil {
		t.Fatalf("Parse(%q) 意外报错: %v", ormTag, err)
	}
	if len(m.Fields) != 1 {
		t.Fatalf("Parse(%q) 期望 1 个字段，得到 %d", ormTag, len(m.Fields))
	}
	return m
}

// parseOne 解析单字段 orm tag，返回该字段 FieldMeta。
func parseOne(t *testing.T, ormTag string) FieldMeta {
	t.Helper()
	return parseOneModel(t, ormTag).Fields[0]
}

// mustParseErr 断言 orm tag 解析报错，且错误信息含字段名、tag 原文与指定片段。
func mustParseErr(t *testing.T, ormTag string, wantContains ...string) error {
	t.Helper()
	st := reflect.StructOf([]reflect.StructField{{
		Name: "F",
		Type: reflect.TypeOf(""),
		Tag:  reflect.StructTag(`orm:"` + ormTag + `"`),
	}})
	_, err := Parse(reflect.New(st).Interface())
	if err == nil {
		t.Fatalf("Parse(%q) 期望报错，实际成功", ormTag)
	}
	msg := err.Error()
	for _, want := range wantContains {
		if !strings.Contains(msg, want) {
			t.Fatalf("Parse(%q) 错误信息缺少 %q:\n%s", ormTag, want, msg)
		}
	}
	if !strings.Contains(msg, `"F"`) {
		t.Fatalf("Parse(%q) 错误信息未含字段名: %s", ormTag, msg)
	}
	if !strings.Contains(msg, ormTag) {
		t.Fatalf("Parse(%q) 错误信息未含 tag 原文: %s", ormTag, msg)
	}
	return err
}

// ── 11.2 逐行矩阵 ────────────────────────────────────────────────────────

// 基础标志 token：各布尔位正确（11.2 第 1 行）。
func TestFlagTokens(t *testing.T) {
	cases := []struct {
		name string
		tag  string
		want FieldMeta
	}{
		{"pk", "pk", FieldMeta{PrimaryKey: true}},
		{"autoincr", "autoincr", FieldMeta{AutoIncrement: true}},
		{"notnull", "notnull", FieldMeta{NotNull: true}},
		{"not null 组合", "not null", FieldMeta{NotNull: true}},
		{"null", "null", FieldMeta{Null: true}},
		{"unique", "unique", FieldMeta{Unique: true}},
		{"index", "index", FieldMeta{Index: true}},
		{"created", "created", FieldMeta{Created: true}},
		{"updated", "updated", FieldMeta{Updated: true}},
		{"version", "version", FieldMeta{Version: true}},
		{"extends", "extends", FieldMeta{Extends: true}},
		{"unsigned", "unsigned", FieldMeta{Unsigned: true}},
		{"utc", "utc", FieldMeta{UTC: true}},
		{"local", "local", FieldMeta{Local: true}},
		{"json", "json", FieldMeta{JSON: true}},
		{"jsonb 同样置位 JSON", "jsonb", FieldMeta{JSON: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseOne(t, tc.tag)
			if got.Ignore || got.ReadOnly || got.WriteOnly || got.ColumnName != "" || got.Type != "" {
				t.Fatalf("%q 不应产生额外语义: %+v", tc.tag, got)
			}
			// 逐布尔位比较（忽略 FieldName/RawTag/FieldIndex）
			got.FieldName, got.RawTag, got.FieldIndex = "", "", nil
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("tag %q:\n got %+v\nwant %+v", tc.tag, got, tc.want)
			}
		})
	}
}

// `-` 忽略字段（11.2 第 1 行）。
func TestIgnoreToken(t *testing.T) {
	f := parseOne(t, "-")
	if !f.Ignore {
		t.Fatalf("orm:\"-\" 应置 Ignore=true: %+v", f)
	}
}

// -> / <- 方向防回归（11.2 最后一行；xorm 语义：-> 只写、<- 只读）。
func TestDirectionAntiRegression(t *testing.T) {
	t.Run("-> 是 WriteOnly（OnlyToDB：写入落库、读回不填）", func(t *testing.T) {
		f := parseOne(t, `varchar(100) 'password' ->`)
		if !f.WriteOnly {
			t.Fatalf("-> 必须置 WriteOnly=true: %+v", f)
		}
		if f.ReadOnly {
			t.Fatalf("-> 不得置 ReadOnly=true（方向写反）: %+v", f)
		}
	})
	t.Run("<- 是 ReadOnly（OnlyFromDB：读回填充、写入忽略）", func(t *testing.T) {
		f := parseOne(t, `varchar(100) 'token' <-`)
		if !f.ReadOnly {
			t.Fatalf("<- 必须置 ReadOnly=true: %+v", f)
		}
		if f.WriteOnly {
			t.Fatalf("<- 不得置 WriteOnly=true（方向写反）: %+v", f)
		}
	})
}

// 列名提取：单引号内下划线/数字（11.2 第 2 行）。
func TestColumnName(t *testing.T) {
	cases := []struct{ tag, want string }{
		{`'user_name'`, "user_name"},
		{`'col_2'`, "col_2"},
		{`'id'`, "id"},
		{`'创建时间'`, "创建时间"},
	}
	for _, tc := range cases {
		if got := parseOne(t, tc.tag).ColumnName; got != tc.want {
			t.Fatalf("%s ColumnName = %q, want %q", tc.tag, got, tc.want)
		}
	}
}

// 类型 token 原文与括号参数（11.2 第 3 行）。
func TestTypeTokenRaw(t *testing.T) {
	cases := []struct{ tag, want string }{
		{"varchar(100)", "varchar(100)"},
		{"decimal(10,2)", "decimal(10,2)"},
		{"decimal(10, 2)", "decimal(10, 2)"}, // 原文（含空格）保留
		{"text", "text"},
		{"bigint", "bigint"},
		{"VARCHAR(16)", "VARCHAR(16)"}, // 大小写不敏感识别，原文保留
		{"pk varchar(16) 'id'", "varchar(16)"},
		{"'id' varchar(16) pk", "varchar(16)"},
	}
	for _, tc := range cases {
		if got := parseOne(t, tc.tag).Type; got != tc.want {
			t.Fatalf("%s Type = %q, want %q", tc.tag, got, tc.want)
		}
	}
}

// 类型与列名顺序无关（文档 4.1；11.2 隐含要求）。
func TestTokenOrderIndependent(t *testing.T) {
	a := parseOne(t, `pk varchar(16) 'id'`)
	b := parseOne(t, `'id' varchar(16) pk`)
	a.FieldName, a.RawTag, a.FieldIndex = b.FieldName, b.RawTag, b.FieldIndex
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("token 顺序不应影响结果:\n %+v\n %+v", a, b)
	}
}

// default 三形态与含空格值（11.2 第 4 行，文档 4.1）。
func TestDefaultForms(t *testing.T) {
	cases := []struct{ tag, want string }{
		{"default 0", "0"},
		{`default 'active'`, "active"},
		{`default 'act ive'`, "act ive"},       // 单引号内空格受保护（文档 2.2 实测）
		{`default 'with space'`, "with space"}, // 含空格值完整
		{"default(0)", "0"},                    // 括号参数形态
		{`default('')`, ""},                    // 空串默认值
		{`default('x,y')`, "x,y"},              // 引号内逗号
		{`varchar(10) 'status' notnull default 0`, "0"},
	}
	for _, tc := range cases {
		f := parseOne(t, tc.tag)
		if !f.HasDefault {
			t.Fatalf("%s 应置 HasDefault=true", tc.tag)
		}
		if f.Default != tc.want {
			t.Fatalf("%s Default = %q, want %q", tc.tag, f.Default, tc.want)
		}
	}
	// default 缺少值 → 报错
	mustParseErr(t, "default", "default 缺少值")
}

// comment 提取，含转义引号（11.2 第 5 行）。
func TestComment(t *testing.T) {
	if got := parseOne(t, `comment('用户昵称')`).Comment; got != "用户昵称" {
		t.Fatalf("Comment = %q", got)
	}
	if got := parseOne(t, `comment('it''s ok')`).Comment; got != "it's ok" {
		t.Fatalf("转义引号 Comment = %q, want it's ok", got)
	}
}

// 联合索引/联合唯一组名（11.2 第 6 行）。
func TestIndexUniqueNames(t *testing.T) {
	f := parseOne(t, "index(idx_org)")
	if !f.Index || f.IndexName != "idx_org" {
		t.Fatalf("index(idx_org): %+v", f)
	}
	f = parseOne(t, "unique(uk_name)")
	if !f.Unique || f.UniqueName != "uk_name" {
		t.Fatalf("unique(uk_name): %+v", f)
	}
	// 组名与列级约束互不干扰
	f = parseOne(t, `varchar(20) 'phone' unique(uk_tenant_phone) default ''`)
	if !f.Unique || f.UniqueName != "uk_tenant_phone" || f.Default != "" || !f.HasDefault {
		t.Fatalf("组合 tag: %+v", f)
	}
}

// extends 前缀参数（11.2 第 11 行；引号去引号）。
func TestExtendsPrefix(t *testing.T) {
	f := parseOne(t, "extends")
	if !f.Extends || f.ExtendsPrefix != "" {
		t.Fatalf("extends: %+v", f)
	}
	f = parseOne(t, `extends('author_')`)
	if !f.Extends || f.ExtendsPrefix != "author_" {
		t.Fatalf("extends('author_'): %+v", f)
	}
}

// 禁用 token：报错且错误信息含替代方案指引（11.2；文档 4.4）。
func TestDisabledTokens(t *testing.T) {
	for _, tok := range []string{"cache", "nocache"} {
		mustParseErr(t, tok,
			"禁用 token", "Query().Cache()", "缓存体系冲突")
	}
	mustParseErr(t, "deleted",
		"禁用 token", "OnlyTrashed/Restore/ForceDelete", "软删除")
	// 禁用 token 混在合法 token 中同样报错
	mustParseErr(t, `varchar(100) 'x' cache`, "禁用 token", "cache")
}

// 未知裸 token：白名单校验报错，含字段名与 tag 原文（11.2；文档 4.4）。
func TestUnknownBareToken(t *testing.T) {
	mustParseErr(t, "serializer:json", "未知裸 token", "serializer:json")
	mustParseErr(t, "pkk", "未知裸 token", "pkk") // 拼写错误
	mustParseErr(t, `varchar(100) 'x' seralzer`, "未知裸 token")
}

// 未加引号的列名裸 token：报错（11.2；文档 4.1 列名强制单引号）。
func TestUnquotedColumnName(t *testing.T) {
	mustParseErr(t, "varchar(100) user_name", "未知裸 token", "user_name")
}

// 非法语法：未闭合引号/括号、逗号在引号/括号外（11.2）。
func TestIllegalSyntax(t *testing.T) {
	mustParseErr(t, `varchar(10) 'abc`, "语法非法", "未闭合的单引号")
	mustParseErr(t, "varchar(100", "语法非法", "未闭合的括号")
	mustParseErr(t, `decimal(10,2), 'x'`, "语法非法", "逗号必须位于引号或括号内")
	mustParseErr(t, "decimal(1(2))", "语法非法")
	mustParseErr(t, "varchar(10))", "语法非法", "多余的右括号")
}

// not null 组合与悬空 not。
func TestNotNullCombination(t *testing.T) {
	f := parseOne(t, `varchar(10) 'x' not null`)
	if !f.NotNull || f.Null {
		t.Fatalf("not null 应置 NotNull=true 且 Null=false: %+v", f)
	}
	// 大小写不敏感
	f = parseOne(t, "NOT NULL")
	if !f.NotNull {
		t.Fatalf("NOT NULL: %+v", f)
	}
	// 悬空 not → 报错
	mustParseErr(t, "varchar(10) not", "NOT token 必须与 NULL 组合")
}

// 大小写不敏感（xorm handler 以大写名注册）。
func TestCaseInsensitiveTokens(t *testing.T) {
	f := parseOne(t, "PK VARCHAR(16) NOTNULL DEFAULT 0")
	if !f.PrimaryKey || !f.NotNull || !f.HasDefault || f.Type != "VARCHAR(16)" {
		t.Fatalf("大写 token: %+v", f)
	}
}

// 空 tag / 无 tag 字段：零值 FieldMeta，不误判（11.2 倒数第 3 行）。
func TestEmptyAndMissingTag(t *testing.T) {
	// ParseField 空 tag → 零值（FieldName 除外）
	f := ParseField(reflect.StructField{Name: "Any"})
	if !reflect.DeepEqual(f, FieldMeta{FieldName: "Any"}) {
		t.Fatalf("空 tag 应为零值 FieldMeta: %+v", f)
	}
	f = ParseField(reflect.StructField{Name: "Any", Tag: `orm:""`})
	if !reflect.DeepEqual(f, FieldMeta{FieldName: "Any"}) {
		t.Fatalf("空 orm tag 应为零值 FieldMeta: %+v", f)
	}

	// 无 orm tag 字段不进入 Fields；Rels/Exts map 非 nil
	st := reflect.StructOf([]reflect.StructField{
		{Name: "A", Type: reflect.TypeOf("")},
		{Name: "B", Type: reflect.TypeOf(0), Tag: reflect.StructTag(`orm:"'b'"`)},
	})
	m, err := Parse(reflect.New(st).Interface())
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Fields) != 1 || m.Fields[0].FieldName != "B" {
		t.Fatalf("无 tag 字段不应收录: %+v", m.Fields)
	}
	if m.Rels == nil || m.Exts == nil {
		t.Fatal("Rels/Exts map 应非 nil")
	}
}

// ParseField 尽力解析：出错时返回已解析部分且不 panic（RawTag 保留原文）。
func TestParseFieldBestEffort(t *testing.T) {
	sf := reflect.StructField{Name: "Bad", Tag: reflect.StructTag(`orm:"cache varchar(10)"`)}
	f := ParseField(sf)
	if f.FieldName != "Bad" || f.RawTag != "cache varchar(10)" {
		t.Fatalf("ParseField 应尽力返回部分结果: %+v", f)
	}
	if !f.Ignore && f.Type != "" && !f.PrimaryKey {
		// 部分结果不强制断言具体位，仅保证非 panic 且 RawTag 可定位
		_ = f
	}
}

// ── 嵌入展开 ─────────────────────────────────────────────────────────────

// 嵌入类型必须导出（文档 2.2/2.3 实测约束：非导出匿名嵌入被双驱动静默跳过），
// 匿名嵌入的字段名即类型名，故嵌入类型名需导出。
type EmbedInner struct {
	Flag bool `orm:"'flag'"`
}
type EmbedMid struct {
	EmbedInner
	Extra int `orm:"'extra'"`
}

type embedOuter struct {
	EmbedMid
	Title string `orm:"varchar(100) 'title'"`
}

// 嵌入 extends 模型：递归展开、FieldIndex 路径正确（11.2 倒数第 4 行）。
func TestAnonymousEmbedExpansion(t *testing.T) {
	m, err := Parse(&embedOuter{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct {
		path  []int
		col   string
		extra func(FieldMeta) bool
	}{
		"Flag":  {[]int{0, 0, 0}, "flag", nil},
		"Extra": {[]int{0, 1}, "extra", nil},
		"Title": {[]int{1}, "title", nil},
	}
	if len(m.Fields) != len(want) {
		t.Fatalf("Fields = %d, want %d: %+v", len(m.Fields), len(want), m.Fields)
	}
	for _, f := range m.Fields {
		w, ok := want[f.FieldName]
		if !ok {
			t.Fatalf("意外字段 %q", f.FieldName)
		}
		if !reflect.DeepEqual(f.FieldIndex, w.path) {
			t.Fatalf("%s FieldIndex = %v, want %v", f.FieldName, f.FieldIndex, w.path)
		}
		if f.ColumnName != w.col {
			t.Fatalf("%s ColumnName = %q, want %q", f.FieldName, f.ColumnName, w.col)
		}
	}
}

type ormAuthorFields struct {
	Name string `orm:"varchar(50) 'name'"`
	ID   string `orm:"pk varchar(16) 'id'"`
}

// extends('前缀')：嵌入标记条目 + 叶子列名加前缀 + 指针嵌入。
func TestExtendsPrefixExpansion(t *testing.T) {
	type post struct {
		Author *ormAuthorFields `orm:"extends('author_')"`
		Title  string           `orm:"varchar(100) 'title'"`
	}
	m, err := Parse(&post{})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]FieldMeta{}
	for _, f := range m.Fields {
		byName[f.FieldName] = f
	}
	marker, ok := byName["Author"]
	if !ok || !marker.Extends || marker.ExtendsPrefix != "author_" {
		t.Fatalf("嵌入标记条目缺失或错误: %+v (all %v)", marker, keys(byName))
	}
	if !reflect.DeepEqual(marker.FieldIndex, []int{0}) {
		t.Fatalf("marker FieldIndex = %v", marker.FieldIndex)
	}
	name := byName["Name"]
	if name.ColumnName != "author_name" || !reflect.DeepEqual(name.FieldIndex, []int{0, 0}) {
		t.Fatalf("叶子前缀列名错误: %+v", name)
	}
	if id := byName["ID"]; id.ColumnName != "author_id" || !id.PrimaryKey {
		t.Fatalf("叶子主键前缀列名错误: %+v", id)
	}
}

// 非导出嵌入类型静默跳过（文档 2.2/2.3 实测语义）。
func TestUnexportedEmbedSkipped(t *testing.T) {
	type hidden struct {
		X string `orm:"'x'"`
	}
	type outer struct {
		hidden
		Title string `orm:"'title'"`
	}
	m, err := Parse(&outer{})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Fields) != 1 || m.Fields[0].FieldName != "Title" {
		t.Fatalf("非导出嵌入应跳过: %+v", m.Fields)
	}
	if !reflect.DeepEqual(m.Fields[0].FieldIndex, []int{1}) {
		t.Fatalf("Title FieldIndex = %v, want [1]", m.Fields[0].FieldIndex)
	}
}

// orm:"-" 的匿名嵌入：整组忽略、不展开（仅保留 Ignore 标记条目）。
func TestIgnoredEmbed(t *testing.T) {
	type outer struct {
		EmbedMid `orm:"-"`
		Title    string `orm:"'title'"`
	}
	m, err := Parse(&outer{})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Fields) != 2 {
		t.Fatalf("Fields = %+v", m.Fields)
	}
	ignore := m.Fields[0]
	if ignore.FieldName != "EmbedMid" || !ignore.Ignore || !reflect.DeepEqual(ignore.FieldIndex, []int{0}) {
		t.Fatalf("嵌入忽略标记错误: %+v", ignore)
	}
}

// 循环嵌入报错（自引用指针嵌入）。
type EmbedCycle struct {
	*EmbedCycle
	Name string `orm:"'name'"`
}

func TestCycleEmbed(t *testing.T) {
	if _, err := Parse(&EmbedCycle{}); err == nil || !strings.Contains(err.Error(), "循环嵌入") {
		t.Fatalf("循环嵌入应报错: %v", err)
	}
}

// ── 缓存与入口校验 ───────────────────────────────────────────────────────

type ormCached struct {
	ID string `orm:"pk varchar(16) 'id'"`
}

// 缓存：同 reflect.Type 两次 Parse 返回同一 *ModelMeta 指针（11.2 倒数第 3 行）。
func TestParseCacheIdempotent(t *testing.T) {
	m1, err := Parse(&ormCached{})
	if err != nil {
		t.Fatal(err)
	}
	m2, err := Parse(&ormCached{})
	if err != nil {
		t.Fatal(err)
	}
	m3, err := Parse(ormCached{}) // 值与指针同 Type
	if err != nil {
		t.Fatal(err)
	}
	pp := &ormCached{}
	m4, err := Parse(&pp) // 多级指针
	if err != nil {
		t.Fatal(err)
	}
	if m1 != m2 || m2 != m3 || m3 != m4 {
		t.Fatal("同类型 Parse 必须返回同一 *ModelMeta 指针")
	}
	if m1.Type != reflect.TypeOf(ormCached{}) {
		t.Fatalf("ModelMeta.Type 错误: %v", m1.Type)
	}
}

// 非法入参。
func TestParseInvalidInput(t *testing.T) {
	if _, err := Parse(nil); err == nil {
		t.Fatal("nil 应报错")
	}
	if _, err := Parse(42); err == nil {
		t.Fatal("非 struct 应报错")
	}
	if _, err := Parse([]ormCached{}); err == nil {
		t.Fatal("切片应报错")
	}
}

// keys 测试辅助。
func keys(m map[string]FieldMeta) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
