package ormtag

import (
	"reflect"
	"strings"
	"testing"
)

// parseExtOne 解析单字段 ext tag，返回 ExtMeta。
func parseExtOne(t *testing.T, extTag string) (ExtMeta, bool) {
	t.Helper()
	st := reflect.StructOf([]reflect.StructField{{
		Name: "F",
		Type: reflect.TypeOf(""),
		Tag:  reflect.StructTag(`orm:"'f'" ext:"` + extTag + `"`),
	}})
	m, err := Parse(reflect.New(st).Interface())
	if err != nil {
		t.Fatalf("Parse ext %q 意外报错: %v", extTag, err)
	}
	em, ok := m.Exts["F"]
	return em, ok
}

// ext 全键（11.2 第 10 行；文档 4.3 表）。
func TestExtAllKeys(t *testing.T) {
	em, ok := parseExtOne(t, "check:age > 13")
	if !ok || em.Check != "age > 13" {
		t.Fatalf("check = %+v (ok=%v)", em, ok)
	}

	em, _ = parseExtOne(t, "index:idx_age,type:hash,where:age > 18,priority:2,sort:desc,length:10")
	if em.IndexOptions != "idx_age,type:hash,where:age > 18,priority:2,sort:desc,length:10" {
		t.Fatalf("IndexOptions 完整选项原文 = %q", em.IndexOptions)
	}

	em, _ = parseExtOne(t, "autoIncrementIncrement:5")
	if em.AutoIncrementIncrement != 5 {
		t.Fatalf("AutoIncrementIncrement = %d", em.AutoIncrementIncrement)
	}

	em, _ = parseExtOne(t, "migration:false")
	if !em.IgnoreMigration {
		t.Fatalf("migration:false 应置 IgnoreMigration: %+v", em)
	}

	em, _ = parseExtOne(t, "timePrecision:milli")
	if em.TimePrecision != "milli" {
		t.Fatalf("TimePrecision = %q", em.TimePrecision)
	}
	em, _ = parseExtOne(t, "timePrecision:nano")
	if em.TimePrecision != "nano" {
		t.Fatalf("TimePrecision(nano) = %q", em.TimePrecision)
	}

	em, _ = parseExtOne(t, "perm:create")
	if !em.PermCreateOnly || em.PermUpdateOnly {
		t.Fatalf("perm:create = %+v", em)
	}
	em, _ = parseExtOne(t, "perm:update")
	if !em.PermUpdateOnly || em.PermCreateOnly {
		t.Fatalf("perm:update = %+v", em)
	}
	em, _ = parseExtOne(t, "perm:create,update")
	if !em.PermCreateOnly || !em.PermUpdateOnly {
		t.Fatalf("perm:create,update = %+v", em)
	}
}

// 多键组合（分号分隔）与大小写不敏感键名。
func TestExtCombinedKeys(t *testing.T) {
	type model struct {
		Age int `orm:"'age' default(0)" ext:"check:age >= 0;autoIncrementIncrement:2;MIGRATION:false"`
	}
	m, err := Parse(&model{})
	if err != nil {
		t.Fatal(err)
	}
	em := m.Exts["Age"]
	if em.Check != "age >= 0" || em.AutoIncrementIncrement != 2 || !em.IgnoreMigration {
		t.Fatalf("组合 ext = %+v", em)
	}
}

// migration 非 false 值 → 不置位。
func TestExtMigrationTrue(t *testing.T) {
	em, _ := parseExtOne(t, "migration:true")
	if em.IgnoreMigration {
		t.Fatalf("migration:true 不应置 IgnoreMigration: %+v", em)
	}
}

// autoIncrementIncrement 非法值报错（错误信息含字段名与 tag 原文）。
func TestExtInvalidAutoIncrementIncrement(t *testing.T) {
	type model struct {
		F string `orm:"'f'" ext:"autoIncrementIncrement:abc"`
	}
	_, err := Parse(&model{})
	if err == nil || !strings.Contains(err.Error(), "autoIncrementIncrement") {
		t.Fatalf("非法步长应报错: %v", err)
	}
	if !strings.Contains(err.Error(), `"F"`) || !strings.Contains(err.Error(), "autoIncrementIncrement:abc") {
		t.Fatalf("错误信息应含字段名与 tag 原文: %v", err)
	}
}

// 未知 ext key 不报错，记录 UnknownKeys 供驱动侧 Warn（文档 11.16）；打了 ext tag 即收录。
func TestExtUnknownKeyIgnored(t *testing.T) {
	em, ok := parseExtOne(t, "foo:bar;serializer:gob")
	if !ok {
		t.Fatal("打了 ext tag 的字段应收录 Exts")
	}
	if em.Check != "" || em.IndexOptions != "" || em.AutoIncrementIncrement != 0 ||
		em.IgnoreMigration || em.TimePrecision != "" || em.PermCreateOnly || em.PermUpdateOnly {
		t.Fatalf("未知键不应产生语义: %+v", em)
	}
	if len(em.UnknownKeys) != 2 || em.UnknownKeys[0] != "foo" || em.UnknownKeys[1] != "serializer" {
		t.Fatalf("未知键应记录到 UnknownKeys（大小写保留、保序）: %v", em.UnknownKeys)
	}
}

// 空 ext tag 不收录。
func TestExtEmptyNotCollected(t *testing.T) {
	type model struct {
		F string `orm:"'f'" ext:""`
		G string `orm:"'g'"`
	}
	m, err := Parse(&model{})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Exts) != 0 {
		t.Fatalf("空/无 ext tag 不应收录: %v", m.Exts)
	}
}

// ext 与 orm index token 并存：ext 只存原文，覆盖关系由驱动侧处理（文档 4.6）。
func TestExtIndexAlongsideOrmIndex(t *testing.T) {
	type model struct {
		Age int `orm:"'age' index(idx_age)" ext:"index:idx_age,where:age > 18"`
	}
	m, err := Parse(&model{})
	if err != nil {
		t.Fatal(err)
	}
	em := m.Exts["Age"]
	if em.IndexOptions != "idx_age,where:age > 18" {
		t.Fatalf("IndexOptions = %q", em.IndexOptions)
	}
	var fm FieldMeta
	for _, f := range m.Fields {
		if f.FieldName == "Age" {
			fm = f
		}
	}
	if !fm.Index || fm.IndexName != "idx_age" {
		t.Fatalf("orm index token 应独立解析: %+v", fm)
	}
}
