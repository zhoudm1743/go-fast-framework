package ormtag

import (
	"fmt"
	"strings"
)

// rel 关联 tag 解析（gorm 风格 key:value; 分号分隔，文档第五章）。
//
// 键全集：foreignKey / references（值可为逗号分隔复合键，解析为 []string，
// 两者等长校验）、many2many、joinForeignKey、joinReferences、polymorphic、
// polymorphicValue、constraint。键名匹配大小写不敏感（对齐 gorm TagSettings）。
//
// 兼容规则（文档 5.3）：rel tag 缺失时回退读 gorm tag 的同名关联键
// （foreignKey/references/many2many/joinForeignKey/joinReferences/
// polymorphic/polymorphicValue，constraint 不在回退范围），
// 解析顺序 rel tag → gorm tag → 约定，保证存量模型零改动。

// gorm 关联键 → rel 键的兼容映射（仅这些键参与 gorm tag 回退）。
var gormAssocKeySet = map[string]bool{
	"foreignkey":       true,
	"references":       true,
	"many2many":        true,
	"joinforeignkey":   true,
	"joinreferences":   true,
	"polymorphic":      true,
	"polymorphicvalue": true,
}

// gormSection 单个 key:value 段。
type gormSection struct {
	key   string // 原样 key（匹配时转小写）
	value string // 第一个冒号后的原文（含可能的冒号，如 constraint:OnDelete:CASCADE）
}

// splitGormSections 按 gorm 风格切分 tag：分号分隔段，每段第一个冒号切分 key/value。
// 无冒号的段忽略（对齐 gorm 对布尔设置之外的裸键的处理）。
func splitGormSections(tagStr string) []gormSection {
	var sections []gormSection
	for _, part := range strings.Split(tagStr, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value := part, ""
		if idx := strings.Index(part, ":"); idx >= 0 {
			key, value = part[:idx], part[idx+1:]
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		sections = append(sections, gormSection{key: key, value: strings.TrimSpace(value)})
	}
	return sections
}

// splitFieldList 逗号分隔的字段名列表（复合键），去除每项首尾空白与空项。
func splitFieldList(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

// parseRelTag 解析 rel tag。返回的 RelMeta 供 ModelMeta.Rels 收录；
// foreignKey 与 references 均存在且数量不等时报错（等长校验，文档 11.2）。
func parseRelTag(fieldName, tagStr string) (RelMeta, error) {
	var meta RelMeta
	for _, sec := range splitGormSections(tagStr) {
		switch strings.ToLower(sec.key) {
		case "foreignkey":
			meta.ForeignKeys = splitFieldList(sec.value)
		case "references":
			meta.References = splitFieldList(sec.value)
		case "many2many":
			meta.Many2Many = sec.value
		case "joinforeignkey":
			meta.JoinForeignKey = sec.value
		case "joinreferences":
			meta.JoinReferences = sec.value
		case "polymorphic":
			meta.Polymorphic = sec.value
		case "polymorphicvalue":
			meta.PolymorphicValue = sec.value
		case "constraint":
			meta.Constraint = sec.value
		default:
			// 未知 rel 键保留为元数据无意义，忽略（消费方按需扩展）
		}
	}
	if err := validateRelPair(fieldName, tagStr, &meta); err != nil {
		return meta, err
	}
	return meta, nil
}

// parseGormAssocTag 从 gorm tag 提取关联元数据（rel tag 缺失时的兼容来源，
// 文档 5.3）。gorm tag 不含任何关联键时 ok=false；constraint 键不参与回退。
func parseGormAssocTag(fieldName, tagStr string) (meta RelMeta, ok bool, err error) {
	if strings.TrimSpace(tagStr) == "" {
		return RelMeta{}, false, nil
	}
	for _, sec := range splitGormSections(tagStr) {
		if !gormAssocKeySet[strings.ToLower(sec.key)] {
			continue
		}
		switch strings.ToLower(sec.key) {
		case "foreignkey":
			meta.ForeignKeys = splitFieldList(sec.value)
		case "references":
			meta.References = splitFieldList(sec.value)
		case "many2many":
			meta.Many2Many = sec.value
		case "joinforeignkey":
			meta.JoinForeignKey = sec.value
		case "joinreferences":
			meta.JoinReferences = sec.value
		case "polymorphic":
			meta.Polymorphic = sec.value
		case "polymorphicvalue":
			meta.PolymorphicValue = sec.value
		}
		ok = true
	}
	if !ok {
		return RelMeta{}, false, nil
	}
	if err := validateRelPair(fieldName, tagStr, &meta); err != nil {
		return meta, true, err
	}
	return meta, true, nil
}

// validateRelPair 校验 foreignKey 与 references 复合键等长（文档 11.2）。
func validateRelPair(fieldName, tagStr string, meta *RelMeta) error {
	if len(meta.ForeignKeys) > 0 && len(meta.References) > 0 &&
		len(meta.ForeignKeys) != len(meta.References) {
		return fmt.Errorf("ormtag: 字段 %q 的 rel tag %q: foreignKey 与 references 数量不等（%d != %d），复合键必须一一对应",
			fieldName, tagStr, len(meta.ForeignKeys), len(meta.References))
	}
	return nil
}
