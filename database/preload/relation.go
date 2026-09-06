package preload

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// ── 关系解析：列级已解析关系 ─────────────────────────────────────────

// Kind 关联方向/形态。
type Kind int

const (
	// HasMany has-many：外键在子表，父行持引用键（回填切片）。
	HasMany Kind = iota
	// HasOne has-one：外键在子表，父行持引用键（回填单个）。
	HasOne
	// BelongsTo belongs-to：外键在父行，子表为被引用表（回填单个）。
	BelongsTo
	// Many2Many 多对多：经中间表两跳查询（回填切片）。
	Many2Many
)

// Relation 列级已解析关系（列名均为数据库裸列名，不含表前缀/schema）。
type Relation struct {
	// Kind 关联方向/形态。
	Kind Kind
	// ChildType 子模型（关联字段元素）结构体类型。
	ChildType reflect.Type
	// FKCols 外键列：has 系在子表；belongs-to 在父行；many2many 为子表侧
	// 连接键（与 JoinRefCols 同源）。
	FKCols []string
	// RefCols 引用列（父行侧）：has 系在父行；belongs-to 在子表；many2many
	// 为父表侧连接键（与 JoinFKCols 同源）。
	RefCols []string
	// Many2Many 中间表表名（仅 Many2Many 时非空）。
	Many2Many string
	// JoinFKCols 中间表指向父的列（仅 Many2Many 时非空）。
	JoinFKCols []string
	// JoinRefCols 中间表指向子的列（仅 Many2Many 时非空）。
	JoinRefCols []string
	// PolyColumn 多态类型列（空 = 非多态；位于子表）。
	PolyColumn string
	// PolyValue 多态类型值（缺省 = 父表名）。
	PolyValue string
}

// Resolver 关联解析器：把（父模型类型, 关联字段名）解析为列级 Relation。
// isSlice 为关联字段是否切片形态（has-many），参与方向探测优先级。
type Resolver interface {
	Resolve(parentType reflect.Type, fieldName string, isSlice bool) (*Relation, error)
}

// ── DefaultResolver：rel tag → gorm tag → 约定 → 方向判定 ────────────

// DefaultResolver 缺省关联解析器（orm-tag-design.md §5.2/5.3）。
// 解析顺序：rel tag → gorm tag（存量兼容）→ 约定回退 + 两侧存在性探测。
// 含 many2many 键直接判定多对多；含 polymorphic 键直接判定多态（优先于
// 方向探测）。前置校验（§9.2 契约 6）：关联字段必须带忽略标记
// （orm:"-" / xorm:"-" / gorm:"-" 任一原始 tag 命中即可；ormtag 解析产物
// FieldMeta.Ignore 的命中由 ORMTagResolver 承担），缺失报 ErrUnsupported。
type DefaultResolver struct {
	Meta MetaAdapter
}

// Resolve 实现 Resolver（见类型注释）。
func (r *DefaultResolver) Resolve(parentType reflect.Type, fieldName string, isSlice bool) (*Relation, error) {
	if r == nil || r.Meta == nil {
		return nil, fmt.Errorf("%w: DefaultResolver 未配置 Meta", contracts.ErrUnsupported)
	}
	field, ok := parentType.FieldByName(fieldName)
	if !ok || field.Anonymous {
		return nil, fmt.Errorf("%w: Preload 字段 %q 在模型 %s 上不存在", contracts.ErrUnsupported, fieldName, parentType.Name())
	}
	childType, _, isRel := relationElemType(field.Type)
	if !isRel {
		return nil, fmt.Errorf("%w: Preload 字段 %q 类型 %s 不是关联类型（需 struct/指针/切片）", contracts.ErrUnsupported, fieldName, field.Type)
	}
	// 契约 6 前置校验：关联字段必须带忽略标记
	if !hasIgnoreMarker(field) {
		return nil, fmt.Errorf("%w: Preload 字段 %q 不是关联字段（关联字段需 orm:\"-\"/xorm:\"-\"/gorm:\"-\" 忽略标记）", contracts.ErrUnsupported, fieldName)
	}
	// 解析顺序（5.3）：rel tag → gorm tag → 约定
	if keys := parseAssocTag(field.Tag.Get("rel")); hasAssocKeys(keys) {
		return resolveTagged(r.Meta, parentType, childType, fieldName, isSlice, keys)
	}
	if keys := parseAssocTag(field.Tag.Get("gorm")); hasAssocKeys(keys) {
		return resolveTagged(r.Meta, parentType, childType, fieldName, isSlice, keys)
	}
	return resolveDirectional(r.Meta, parentType, childType, fieldName, isSlice, nil, nil)
}

// ── 标签解析（rel 与 gorm tag 共用，gorm 风格 key:value; 分号分隔）────

// assocTagKeys 参与关联解析的标签键全集（rel tag 与 gorm 关联 tag 同名同义）。
var assocTagKeys = []string{
	"foreignKey", "references", "many2many", "joinForeignKey",
	"joinReferences", "polymorphic", "polymorphicValue",
}

// parseAssocTag 解析 gorm 风格 `key:value;` 标签为键值 map。
func parseAssocTag(tag string) map[string]string {
	out := make(map[string]string)
	for _, part := range strings.Split(tag, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, _ := strings.Cut(part, ":")
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out
}

// hasAssocKeys 判定标签键值中是否含任一关联键（gorm tag 存量兼容探测用）。
func hasAssocKeys(keys map[string]string) bool {
	for _, k := range assocTagKeys {
		if keys[k] != "" {
			return true
		}
	}
	return false
}

// hasIgnoreMarker 契约 6 忽略标记检测：orm / xorm / gorm 原始 tag 值
// 精确为 "-"（三种标记任一命中即可）。
func hasIgnoreMarker(field reflect.StructField) bool {
	for _, key := range []string{"orm", "xorm", "gorm"} {
		if strings.TrimSpace(field.Tag.Get(key)) == "-" {
			return true
		}
	}
	return false
}

// splitCSV 逗号分隔的 Go 字段名列表（复合键支持）。
func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// resolveTagged 按标签键组装 Relation：many2many → 多对多；polymorphic →
// 多态（优先级高于方向探测，5.2）；否则进入方向判定（显式 foreignKey/
// references 参与）。
func resolveTagged(meta MetaAdapter, parentType, childType reflect.Type, fieldName string, isSlice bool, keys map[string]string) (*Relation, error) {
	if m2m := keys["many2many"]; m2m != "" {
		return resolveMany2Many(meta, parentType, childType, m2m, splitCSV(keys["joinForeignKey"]), splitCSV(keys["joinReferences"]))
	}
	if poly := keys["polymorphic"]; poly != "" {
		return resolvePolymorphic(meta, parentType, childType, isSlice, poly, keys["polymorphicValue"], splitCSV(keys["foreignKey"]), splitCSV(keys["references"]))
	}
	return resolveDirectional(meta, parentType, childType, fieldName, isSlice, splitCSV(keys["foreignKey"]), splitCSV(keys["references"]))
}

// resolveMany2Many 组装多对多 Relation：joinForeignKey/joinReferences 为
// 父/子模型上的 Go 字段名，经 Meta 列名映射；缺省回退父/子表主键。
func resolveMany2Many(meta MetaAdapter, parentType, childType reflect.Type, joinTable string, joinFkNames, joinRefNames []string) (*Relation, error) {
	rel := &Relation{Kind: Many2Many, ChildType: childType, Many2Many: joinTable}
	joinFKCols, err := columnsOfOrDefault(meta, parentType, joinFkNames)
	if err != nil {
		return nil, err
	}
	joinRefCols, err := columnsOfOrDefault(meta, childType, joinRefNames)
	if err != nil {
		return nil, err
	}
	rel.JoinFKCols = joinFKCols
	rel.JoinRefCols = joinRefCols
	rel.RefCols = append([]string(nil), joinFKCols...) // 父表侧连接键（父行取值列）
	rel.FKCols = append([]string(nil), joinRefCols...) // 子表侧连接键（子行分组列）
	return rel, nil
}

// resolvePolymorphic 组装多态 Relation：子表外键缺省为 <poly>ID 字段映射列
// （约定回退 <poly 蛇形>_id），类型列为 <poly>Type 映射列（约定回退
// <poly 蛇形>_type）；引用列缺省父主键；PolyValue 缺省父表名（契约 8）。
func resolvePolymorphic(meta MetaAdapter, parentType, childType reflect.Type, isSlice bool, poly, polyValue string, fkNames, refNames []string) (*Relation, error) {
	rel := &Relation{ChildType: childType}
	if isSlice {
		rel.Kind = HasMany
	} else {
		rel.Kind = HasOne
	}
	refs, err := columnsOfOrDefault(meta, parentType, refNames)
	if err != nil {
		return nil, err
	}
	rel.RefCols = refs
	switch {
	case len(fkNames) > 0:
		fks, err := columnsOf(meta, childType, fkNames)
		if err != nil {
			return nil, err
		}
		rel.FKCols = fks
	default:
		if col, ok := meta.ColumnOfField(childType, poly+"ID"); ok {
			rel.FKCols = []string{col}
		} else {
			rel.FKCols = []string{toSnake(poly) + "_id"}
		}
	}
	if len(rel.FKCols) != len(rel.RefCols) {
		return nil, fmt.Errorf("%w: 多态关联 %q 外键列 %v 与引用列 %v 数量不一致", contracts.ErrUnsupported, poly, rel.FKCols, rel.RefCols)
	}
	if col, ok := meta.ColumnOfField(childType, poly+"Type"); ok {
		rel.PolyColumn = col
	} else {
		rel.PolyColumn = toSnake(poly) + "_type"
	}
	if polyValue == "" {
		if polyValue, err = meta.TableName(parentType); err != nil {
			return nil, fmt.Errorf("preload: 解析父模型 %s 表名失败（polymorphicValue 缺省回退）: %w", parentType.Name(), err)
		}
	}
	rel.PolyValue = polyValue
	return rel, nil
}

// resolveDirectional 方向判定 + 约定回退（gofast-xorm resolvePreloadKeys 蓝本）：
// 按"外键列在子表还是父行"两侧存在性探测，切片字段优先探测 has-many、
// 单 struct 字段优先探测 belongs-to（与 gorm 关联推断一致）。
//   - 显式 foreignKey/references（Go 字段名）映射失败即该方向不成立，尝试另一方向；
//   - references 缺省回退引用表主键；
//   - foreignKey 缺省约定回退：has 系 `<父表名蛇形>_<引用列蛇形>`、
//     belongs-to `<关联字段名蛇形>_<引用列蛇形>`；
//   - 两个方向均不成立 → ErrUnsupported 包装（信息含模型名与字段名引导）。
func resolveDirectional(meta MetaAdapter, parentType, childType reflect.Type, fieldName string, isSlice bool, fkNames, refNames []string) (*Relation, error) {
	parentTable, err := meta.TableName(parentType)
	if err != nil {
		return nil, fmt.Errorf("preload: 解析父模型 %s 表名失败: %w", parentType.Name(), err)
	}
	type keyCandidate struct {
		belongsTo bool
		fkType    reflect.Type // 外键所在模型类型
		refType   reflect.Type // 引用键所在模型类型
		defaultFk func(refCol string) string
	}
	candidates := []keyCandidate{
		{false, childType, parentType, func(rc string) string { return toSnake(parentTable) + "_" + toSnake(rc) }},
		{true, parentType, childType, func(rc string) string { return toSnake(fieldName) + "_" + toSnake(rc) }},
	}
	// 方向探测顺序对齐 gorm 推断：切片 → has-many 优先；单 struct → belongs-to 优先
	if !isSlice {
		candidates[0], candidates[1] = candidates[1], candidates[0]
	}
	for _, cand := range candidates {
		refCols, refErr := columnsOfOrDefault(meta, cand.refType, refNames)
		if refErr != nil {
			continue // 该方向 references 字段不存在，尝试另一方向
		}
		var fkCols []string
		if len(fkNames) > 0 {
			fkCols, err = columnsOf(meta, cand.fkType, fkNames)
			if err != nil {
				continue // 该方向 foreignKey 字段不存在，尝试另一方向
			}
		} else {
			for _, rc := range refCols {
				fkCols = append(fkCols, cand.defaultFk(rc))
			}
		}
		if len(fkCols) == 0 || len(fkCols) != len(refCols) {
			continue
		}
		valid := true
		for i := range fkCols {
			if !meta.HasColumn(cand.fkType, fkCols[i]) || !meta.HasColumn(cand.refType, refCols[i]) {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		kind := HasOne
		if isSlice {
			kind = HasMany
		}
		if cand.belongsTo {
			kind = BelongsTo
		}
		return &Relation{Kind: kind, ChildType: childType, FKCols: fkCols, RefCols: refCols}, nil
	}
	return nil, fmt.Errorf("%w: Preload 字段 %q 无法在模型 %s 与 %s 间解析关联键与方向（可显式书写 rel tag 的 foreignKey/references，或改用 Joins）",
		contracts.ErrUnsupported, fieldName, parentType.Name(), childType.Name())
}

// columnsOfOrDefault 解析引用键列：字段名缺省时回退模型主键；字段名存在
// 但映射失败报 ErrUnsupported（由调用方按方向探测语义决定跳过或透出）。
func columnsOfOrDefault(meta MetaAdapter, t reflect.Type, fieldNames []string) ([]string, error) {
	if len(fieldNames) == 0 {
		pks, err := meta.PrimaryKeys(t)
		if err != nil {
			return nil, fmt.Errorf("preload: 解析模型 %s 主键失败: %w", t.Name(), err)
		}
		if len(pks) == 0 {
			return nil, fmt.Errorf("%w: 模型 %s 无主键可作引用列（请显式指定 references）", contracts.ErrUnsupported, t.Name())
		}
		return pks, nil
	}
	return columnsOf(meta, t, fieldNames)
}

// columnsOf 将 Go 字段名列表映射为列名列表；任一字段无对应列即报
// ErrUnsupported（信息含模型名与字段名）。
func columnsOf(meta MetaAdapter, t reflect.Type, fieldNames []string) ([]string, error) {
	cols := make([]string, 0, len(fieldNames))
	for _, name := range fieldNames {
		col, ok := meta.ColumnOfField(t, name)
		if !ok {
			return nil, fmt.Errorf("%w: 字段 %q 在模型 %s 中不存在或未映射为列", contracts.ErrUnsupported, name, t.Name())
		}
		cols = append(cols, col)
	}
	return cols, nil
}

// toSnake 驼峰/帕斯卡命名转蛇形（约定回退用，自实现不引第三方库）。
// "UserID"→"user_id"、"DeptID"→"dept_id"、"HTTPServer"→"http_server"，
// 已蛇形输入幂等。
func toSnake(s string) string {
	runes := []rune(s)
	var b strings.Builder
	b.Grow(len(runes) + 4)
	for i, r := range runes {
		if r >= 'A' && r <= 'Z' {
			prevLowerOrDigit := i > 0 && (runes[i-1] >= 'a' && runes[i-1] <= 'z' || runes[i-1] >= '0' && runes[i-1] <= '9')
			nextLower := i+1 < len(runes) && runes[i+1] >= 'a' && runes[i+1] <= 'z'
			if i > 0 && (prevLowerOrDigit || nextLower && runes[i-1] >= 'A' && runes[i-1] <= 'Z') {
				b.WriteByte('_')
			}
			b.WriteRune(r - 'A' + 'a')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
