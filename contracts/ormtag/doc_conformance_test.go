package ormtag

import (
	"reflect"
	"testing"
)

// 临时一致性冒烟：设计文档附录 A 速查表模型逐字段解析。
func TestDocAppendixAModel(t *testing.T) {
	type AuditFields struct {
		CreatedAt int64 `orm:"created 'created_at'"`
		UpdatedAt int64 `orm:"updated 'updated_at'"`
	}
	type AuthorFields struct {
		AuthorName string `orm:"varchar(50) 'name'"`
	}
	type Order struct{}
	type Role struct{}
	type Photo struct{}

	type Example struct {
		ID        string       `orm:"pk varchar(16) 'id'"`
		Seq       int64        `orm:"pk autoincr 'seq'"`
		Name      string       `orm:"varchar(100) 'name' notnull default('') comment('姓名')"`
		Email     string       `orm:"varchar(100) 'email' unique"`
		TenantID  string       `orm:"varchar(16) 'tenant_id' unique(uk_tenant_phone)"`
		Phone     string       `orm:"varchar(20) 'phone' unique(uk_tenant_phone)"`
		OrgID     string       `orm:"varchar(16) 'org_id' index(idx_org_status)"`
		Status    int          `orm:"'status' index(idx_org_status) default(0)"`
		Amount    float64      `orm:"decimal(10,2) 'amount'"`
		CreatedAt int64        `orm:"created 'created_at'"`
		UpdatedAt int64        `orm:"updated 'updated_at'"`
		DeletedAt int64        `orm:"'deleted_at' index default(0)"`
		Password  string       `orm:"varchar(100) 'password' ->"`
		Token     string       `orm:"varchar(64) 'token' <-"`
		Internal  string       `orm:"-"`
		Level     uint         `orm:"int unsigned 'level' default(0)"`
		Audit     AuditFields  `orm:"extends"`
		Author    AuthorFields `orm:"extends('author_')"`
		Orders    []Order      `orm:"-" rel:"foreignKey:UserID;references:ID"`
		Roles     []Role       `orm:"-" rel:"many2many:user_roles;joinForeignKey:ID;joinReferences:ID"`
		Photos    []Photo      `orm:"-" rel:"polymorphic:Owner;polymorphicValue:users"`
		Version   int          `orm:"version 'version'"`
		Age       int          `orm:"'age' default(0)" ext:"check:age >= 0"`
		Email2    string       `orm:"varchar(100) 'email2'" ext:"index:idx_email2_active,where:status = 1"`
	}

	m, err := Parse(&Example{})
	if err != nil {
		t.Fatalf("附录 A 模型解析失败: %v", err)
	}
	byName := map[string]FieldMeta{}
	for _, f := range m.Fields {
		byName[f.FieldName] = f
	}
	checks := []struct {
		field  string
		assert func(FieldMeta) bool
	}{
		{"ID", func(f FieldMeta) bool { return f.PrimaryKey && f.Type == "varchar(16)" && f.ColumnName == "id" }},
		{"Seq", func(f FieldMeta) bool { return f.PrimaryKey && f.AutoIncrement }},
		{"Name", func(f FieldMeta) bool { return f.NotNull && f.HasDefault && f.Default == "" && f.Comment == "姓名" }},
		{"Email", func(f FieldMeta) bool { return f.Unique && f.UniqueName == "" }},
		{"TenantID", func(f FieldMeta) bool { return f.UniqueName == "uk_tenant_phone" }},
		{"Phone", func(f FieldMeta) bool { return f.UniqueName == "uk_tenant_phone" }},
		{"OrgID", func(f FieldMeta) bool { return f.IndexName == "idx_org_status" }},
		{"Status", func(f FieldMeta) bool { return f.IndexName == "idx_org_status" && f.Default == "0" }},
		{"Amount", func(f FieldMeta) bool { return f.Type == "decimal(10,2)" }},
		{"Password", func(f FieldMeta) bool { return f.WriteOnly && !f.ReadOnly }},
		{"Token", func(f FieldMeta) bool { return f.ReadOnly && !f.WriteOnly }},
		{"Internal", func(f FieldMeta) bool { return f.Ignore }},
		{"Level", func(f FieldMeta) bool { return f.Type == "int" && f.Unsigned }},
		{"Version", func(f FieldMeta) bool { return f.Version }},
		{"AuthorName", func(f FieldMeta) bool { return f.ColumnName == "author_name" }},
	}
	for _, c := range checks {
		f, ok := byName[c.field]
		if !ok {
			t.Fatalf("缺少字段 %q（Fields=%d）", c.field, len(m.Fields))
		}
		if !c.assert(f) {
			t.Fatalf("%q 断言失败: %+v", c.field, f)
		}
	}
	if len(m.Rels) != 3 || len(m.Exts) != 2 {
		t.Fatalf("Rels=%d Exts=%d", len(m.Rels), len(m.Exts))
	}
	if rm := m.Rels["Photos"]; rm.Polymorphic != "Owner" || rm.PolymorphicValue != "users" {
		t.Fatalf("polymorphic rel: %+v", rm)
	}
	if !reflect.DeepEqual(m.Rels["Roles"].ForeignKeys, []string(nil)) && len(m.Rels["Roles"].ForeignKeys) != 0 {
		t.Fatalf("many2many 无 foreignKey 键时应为空: %+v", m.Rels["Roles"])
	}
}

// 文档 10.1 Model 改造三 tag 过渡形态。
func TestDocModelWithSoftDelete(t *testing.T) {
	type Model struct {
		ID        string `orm:"pk varchar(16) 'id'"`
		CreatedAt int64  `orm:"created 'created_at'"`
		UpdatedAt int64  `orm:"updated 'updated_at'"`
	}
	type SoftDelete struct {
		DeletedAt int64 `orm:"'deleted_at' index default(0)"`
	}
	type ModelWithSoftDelete struct {
		Model      `orm:"extends"`
		SoftDelete `orm:"extends"`
	}
	m, err := Parse(&ModelWithSoftDelete{})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]FieldMeta{}
	for _, f := range m.Fields {
		byName[f.FieldName] = f
	}
	if f := byName["ID"]; !f.PrimaryKey || f.ColumnName != "id" || !reflect.DeepEqual(f.FieldIndex, []int{0, 0}) {
		t.Fatalf("ID: %+v", f)
	}
	if f := byName["DeletedAt"]; f.ColumnName != "deleted_at" || !f.Index || f.Default != "0" || !reflect.DeepEqual(f.FieldIndex, []int{1, 0}) {
		t.Fatalf("DeletedAt: %+v", f)
	}
	_, ok := byName["Model"]
	mk, ok2 := byName["SoftDelete"]
	if !ok || !mk.Extends || !ok2 {
		t.Fatalf("extends 标记条目缺失: %+v", byName)
	}
}
