package preload

import (
	"fmt"
	"reflect"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/contracts/ormtag"
)

// ── ORMTagResolver：基于 contracts/ormtag 统一解析产物 ───────────────

// ORMTagResolver 基于 ormtag.Parse 解析产物的关联解析器。
// 优先消费 ModelMeta.Rels（rel tag 与 gorm tag 兼容读取已由 ormtag 完成，
// 文档 5.3）：many2many → 多对多；polymorphic → 多态（polymorphicValue
// 缺省回退父表名）；其余 foreignKey/references → 方向判定。字段无 rel
// 条目时回退 DefaultResolver（gorm tag → 约定 → 方向探测）。
//
// 契约 6 前置校验：ormtag FieldMeta.Ignore（orm:"-"）或原始 tag
// xorm:"-"/gorm:"-" 任一命中即可，缺失报 ErrUnsupported（含字段名）。
type ORMTagResolver struct {
	// Meta 驱动注入的模型元数据能力。
	Meta MetaAdapter
	// Fallback 无 rel 条目时的回退解析器（可选；nil 时自动构造
	// DefaultResolver{Meta: r.Meta}）。
	Fallback *DefaultResolver
}

// Resolve 实现 Resolver（见类型注释）。
func (r *ORMTagResolver) Resolve(parentType reflect.Type, fieldName string, isSlice bool) (*Relation, error) {
	if r == nil || r.Meta == nil {
		return nil, fmt.Errorf("%w: ORMTagResolver 未配置 Meta", contracts.ErrUnsupported)
	}
	meta, err := ormtag.Parse(reflect.New(parentType).Interface())
	if err != nil {
		return nil, fmt.Errorf("preload: ormtag 解析模型 %s 失败: %w", parentType.Name(), err)
	}
	relMeta, ok := meta.Rels[fieldName]
	if !ok {
		fb := r.Fallback
		if fb == nil {
			fb = &DefaultResolver{Meta: r.Meta}
		}
		return fb.Resolve(parentType, fieldName, isSlice)
	}
	field, ok := parentType.FieldByName(fieldName)
	if !ok || field.Anonymous {
		return nil, fmt.Errorf("%w: Preload 字段 %q 在模型 %s 上不存在", contracts.ErrUnsupported, fieldName, parentType.Name())
	}
	childType, _, isRel := relationElemType(field.Type)
	if !isRel {
		return nil, fmt.Errorf("%w: Preload 字段 %q 类型 %s 不是关联类型（需 struct/指针/切片）", contracts.ErrUnsupported, fieldName, field.Type)
	}
	// 契约 6 前置校验：ormtag FieldMeta.Ignore 或原始 xorm/gorm "-" 任一命中
	if !ormtagIgnored(meta, fieldName) && !hasIgnoreMarker(field) {
		return nil, fmt.Errorf("%w: Preload 字段 %q 不是关联字段（关联字段需 orm:\"-\"/xorm:\"-\"/gorm:\"-\" 忽略标记）", contracts.ErrUnsupported, fieldName)
	}

	switch {
	case relMeta.Many2Many != "":
		// joinForeignKey/joinReferences 为父/子模型 Go 字段名，经 Meta 转
		// 列名；缺省回退父/子主键（resolveMany2Many 内处理）
		var joinFk, joinRef []string
		if relMeta.JoinForeignKey != "" {
			joinFk = []string{relMeta.JoinForeignKey}
		}
		if relMeta.JoinReferences != "" {
			joinRef = []string{relMeta.JoinReferences}
		}
		return resolveMany2Many(r.Meta, parentType, childType, relMeta.Many2Many, joinFk, joinRef)
	case relMeta.Polymorphic != "":
		return resolvePolymorphic(r.Meta, parentType, childType, isSlice, relMeta.Polymorphic, relMeta.PolymorphicValue, relMeta.ForeignKeys, relMeta.References)
	default:
		return resolveDirectional(r.Meta, parentType, childType, fieldName, isSlice, relMeta.ForeignKeys, relMeta.References)
	}
}

// ormtagIgnored 查询 ormtag 解析产物中字段是否带忽略标记（orm:"-" →
// FieldMeta.Ignore）。
func ormtagIgnored(meta *ormtag.ModelMeta, fieldName string) bool {
	for _, f := range meta.Fields {
		if f.FieldName == fieldName {
			return f.Ignore
		}
	}
	return false
}
