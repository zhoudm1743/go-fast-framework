package ormtag

import (
	"reflect"
	"strings"
	"testing"
)

type ormRelUser struct {
	ID string `orm:"pk varchar(16) 'id'"`
}
type ormRelDept struct {
	ID string `orm:"pk varchar(16) 'id'"`
}

// parseRelTestModel 解析带 rel/gorm tag 的关联字段模型。
func parseRelTestModel(t *testing.T, model any) *ModelMeta {
	t.Helper()
	m, err := Parse(model)
	if err != nil {
		t.Fatalf("Parse 意外报错: %v", err)
	}
	return m
}

// rel tag 基础键：foreignKey/references（11.2 第 7 行）。
func TestRelBasicKeys(t *testing.T) {
	type model struct {
		ormRelUser
		Orders []struct{} `orm:"-" rel:"foreignKey:UserID;references:ID"`
	}
	m := parseRelTestModel(t, &model{})
	rm, ok := m.Rels["Orders"]
	if !ok {
		t.Fatalf("Rels 应含 Orders: %v", m.Rels)
	}
	if len(rm.ForeignKeys) != 1 || rm.ForeignKeys[0] != "UserID" {
		t.Fatalf("ForeignKeys = %v", rm.ForeignKeys)
	}
	if len(rm.References) != 1 || rm.References[0] != "ID" {
		t.Fatalf("References = %v", rm.References)
	}
	// rel-only 字段不进入 Fields（Fields 仅收 orm tag 字段，该字段 orm:"-" 亦收录为忽略条目）
	found := false
	for _, f := range m.Fields {
		if f.FieldName == "Orders" {
			found = true
			if !f.Ignore {
				t.Fatalf("Orders 应为忽略字段: %+v", f)
			}
		}
	}
	if !found {
		t.Fatal("orm:\"-\" 的关联字段应收录 Fields（Ignore=true）")
	}
}

// rel 复合键逗号分隔（11.2 第 7 行；文档 5.1）。
func TestRelCompositeKeys(t *testing.T) {
	type model struct {
		ormRelUser
		Orders []struct{} `orm:"-" rel:"foreignKey:OrgID,DeptID;references:ID,TenantID"`
	}
	rm := parseRelTestModel(t, &model{}).Rels["Orders"]
	if !reflect.DeepEqual(rm.ForeignKeys, []string{"OrgID", "DeptID"}) {
		t.Fatalf("ForeignKeys = %v", rm.ForeignKeys)
	}
	if !reflect.DeepEqual(rm.References, []string{"ID", "TenantID"}) {
		t.Fatalf("References = %v", rm.References)
	}
	// 逗号后带空格同样支持
	type model2 struct {
		ormRelUser
		Orders []struct{} `orm:"-" rel:"foreignKey:OrgID, DeptID;references:ID, TenantID"`
	}
	rm2 := parseRelTestModel(t, &model2{}).Rels["Orders"]
	if !reflect.DeepEqual(rm2.ForeignKeys, []string{"OrgID", "DeptID"}) {
		t.Fatalf("空格复合键 ForeignKeys = %v", rm2.ForeignKeys)
	}
}

// 复合键等长校验：数量不等报错（11.2 第 7 行）。
func TestRelCompositeLengthMismatch(t *testing.T) {
	type model struct {
		ormRelUser
		Orders []struct{} `orm:"-" rel:"foreignKey:OrgID,DeptID;references:ID"`
	}
	_, err := Parse(&model{})
	if err == nil || !strings.Contains(err.Error(), "数量不等") {
		t.Fatalf("复合键不等长应报错: %v", err)
	}
	if !strings.Contains(err.Error(), `"Orders"`) {
		t.Fatalf("错误信息应含字段名: %v", err)
	}
}

// 缺省 references：不报错，References 为空（回退对方主键由消费方处理，文档 5.2）。
func TestRelMissingReferences(t *testing.T) {
	type model struct {
		ormRelUser
		Orders []struct{} `orm:"-" rel:"foreignKey:UserID"`
	}
	rm := parseRelTestModel(t, &model{}).Rels["Orders"]
	if len(rm.ForeignKeys) != 1 || rm.ForeignKeys[0] != "UserID" {
		t.Fatalf("ForeignKeys = %v", rm.ForeignKeys)
	}
	if len(rm.References) != 0 {
		t.Fatalf("缺省 references 应为空: %v", rm.References)
	}
}

// rel 扩展键：many2many/joinForeignKey/joinReferences（11.2 第 8 行）。
func TestRelMany2Many(t *testing.T) {
	type model struct {
		ormRelUser
		Roles []struct{} `orm:"-" rel:"many2many:user_languages;joinForeignKey:ID;joinReferences:ID"`
	}
	rm := parseRelTestModel(t, &model{}).Rels["Roles"]
	if rm.Many2Many != "user_languages" {
		t.Fatalf("Many2Many = %q", rm.Many2Many)
	}
	if rm.JoinForeignKey != "ID" || rm.JoinReferences != "ID" {
		t.Fatalf("JoinFK = %q, JoinRef = %q", rm.JoinForeignKey, rm.JoinReferences)
	}
}

// rel 扩展键：polymorphic/polymorphicValue（11.2 第 9 行）。
func TestRelPolymorphic(t *testing.T) {
	type model struct {
		ormRelUser
		Photos []struct{} `orm:"-" rel:"polymorphic:Owner;polymorphicValue:users"`
		Posts  []struct{} `orm:"-" rel:"polymorphic:Owner"` // 缺省 polymorphicValue
	}
	m := parseRelTestModel(t, &model{})
	if rm := m.Rels["Photos"]; rm.Polymorphic != "Owner" || rm.PolymorphicValue != "users" {
		t.Fatalf("Photos rel = %+v", rm)
	}
	// 缺省 polymorphicValue 为空串（回退父表名由消费方处理，文档 5.1）
	if rm := m.Rels["Posts"]; rm.Polymorphic != "Owner" || rm.PolymorphicValue != "" {
		t.Fatalf("Posts rel = %+v", rm)
	}
}

// constraint 键原文保留（值可含冒号/逗号，文档 5.1）。
func TestRelConstraint(t *testing.T) {
	type model struct {
		ormRelUser
		Orders []struct{} `orm:"-" rel:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL"`
	}
	rm := parseRelTestModel(t, &model{}).Rels["Orders"]
	if rm.Constraint != "OnUpdate:CASCADE,OnDelete:SET NULL" {
		t.Fatalf("Constraint = %q", rm.Constraint)
	}
}

// 兼容规则（文档 5.3）：rel tag 缺失时回退读 gorm tag 同名键。
func TestRelGormFallback(t *testing.T) {
	type model struct {
		ormRelUser
		Orders    []struct{} `orm:"-" gorm:"foreignKey:UserID;references:ID"`
		Languages []struct{} `orm:"-" gorm:"many2many:user_languages"`
		OnlyCon   []struct{} `orm:"-" gorm:"constraint:OnDelete:CASCADE"` // 仅 constraint 无关联键 → 不收录
	}
	m := parseRelTestModel(t, &model{})

	rm, ok := m.Rels["Orders"]
	if !ok {
		t.Fatalf("gorm tag 兼容读取失败: %v", m.Rels)
	}
	if rm.ForeignKeys[0] != "UserID" || rm.References[0] != "ID" {
		t.Fatalf("gorm 兼容 rel = %+v", rm)
	}
	if rm := m.Rels["Languages"]; rm.Many2Many != "user_languages" {
		t.Fatalf("gorm many2many 兼容 = %+v", rm)
	}
	if _, ok := m.Rels["OnlyCon"]; ok {
		t.Fatal("gorm tag 仅含 constraint 键不应触发兼容收录（constraint 不在回退范围）")
	}

	// polymorphic 兼容
	type model2 struct {
		ormRelUser
		Photos []struct{} `orm:"-" gorm:"polymorphic:Owner"`
	}
	if rm := parseRelTestModel(t, &model2{}).Rels["Photos"]; rm.Polymorphic != "Owner" {
		t.Fatalf("gorm polymorphic 兼容 = %+v", rm)
	}
}

// 优先级：rel tag 存在时完全取代 gorm tag（文档 5.3 解析顺序）。
func TestRelPrecedenceOverGorm(t *testing.T) {
	type model struct {
		ormRelUser
		Orders []struct{} `orm:"-" rel:"foreignKey:A" gorm:"foreignKey:B;references:ID"`
	}
	rm := parseRelTestModel(t, &model{}).Rels["Orders"]
	if !reflect.DeepEqual(rm.ForeignKeys, []string{"A"}) {
		t.Fatalf("rel tag 应优先: %+v", rm)
	}
	if len(rm.References) != 0 {
		t.Fatalf("不得混入 gorm tag 的 references: %+v", rm)
	}
}

// rel 复合键不等长在 gorm 兼容路径同样报错。
func TestRelGormFallbackMismatch(t *testing.T) {
	type model struct {
		ormRelUser
		Orders []struct{} `orm:"-" gorm:"foreignKey:OrgID,DeptID;references:ID"`
	}
	_, err := Parse(&model{})
	if err == nil || !strings.Contains(err.Error(), "数量不等") {
		t.Fatalf("gorm 兼容路径等长校验失败: %v", err)
	}
}
