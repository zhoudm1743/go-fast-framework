// Package preload 实现驱动无关的共享关联预加载（Preload）引擎。
//
// 引擎只依赖 contracts.Query 抽象与驱动注入的 MetaAdapter 元数据能力，
// 不 import 任何具体 ORM/SQL 驱动；表名/主键/列名解析一律经 MetaAdapter
// （裸表名，schema/前缀由 NewQuery 闭包 baked in），子查询由 NewQuery 创建
// （继承 schema/ctx/事务/缓存配置，不继承父链条件，契约 §9.2-8）。
//
// 行为契约（orm-tag-design.md §9.2，9 条，全部入 engine_test.go）：
//  1. 批量 IN 子查询非 N+1；复合外键手写元组 IN 并分批防超参；
//  2. 嵌套路径 "Orders.Items" 以本层回填后的子行作为下层父行递归；
//  3. conds 作用于首层子查询；callbacks 在子查询构造后、Find 前依次应用；
//     many2many 时 conds/callbacks 作用于子表查询（第二跳）；
//  4. 父行引用键任一零值 → 跳过该行，关联字段保持零值；全部为零值/空父行
//     时不发起查询；
//  5. 无子行 → has-many 回填非 nil 空切片、has-one/belongs-to 保持零值；
//  6. 关联字段必须带忽略标记，缺失报 ErrUnsupported（Resolver 前置校验）；
//  7. many2many 两跳：先查中间表映射行，再以引用键集 IN 查子表按映射回填；
//     中间表模型无需业务定义（引擎直接构造表级子查询）；
//  8. polymorphic：子查询追加类型列 = PolyValue 条件，缺省值取父表名；
//  9. 引擎对中间表只读（关联管理不写中间表）。
package preload

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// compositeBatchSize 复合外键元组 IN 每批元组数（防占位符超参，契约 1）。
const compositeBatchSize = 500

// MetaAdapter 驱动注入的模型元数据能力（表名/主键/列名解析）。
// 返回的表名/列名一律为数据库裸名（不含前缀/schema）。
type MetaAdapter interface {
	// TableName 模型类型对应的数据库表名。
	TableName(modelType reflect.Type) (string, error)
	// PrimaryKeys 模型类型的主键列名列表。
	PrimaryKeys(modelType reflect.Type) ([]string, error)
	// ColumnOfField 模型上 Go 字段名映射的列名（含嵌入展开由实现方负责）。
	ColumnOfField(modelType reflect.Type, fieldName string) (string, bool)
	// HasColumn 列名是否存在于模型对应表中。
	HasColumn(modelType reflect.Type, columnName string) bool
}

// Engine 共享 Preload 引擎。
type Engine struct {
	// Meta 驱动注入的模型元数据能力。
	Meta MetaAdapter
	// NewQuery 驱动注入：创建子查询（继承 schema/ctx/tx/缓存配置，
	// 但不继承父链条件）。
	NewQuery func() contracts.Query
	// Resolver 关联解析器（可选；缺省 DefaultResolver{Meta: e.Meta}）。
	// 驱动可注入 ORMTagResolver 以优先消费 ormtag 解析产物。
	Resolver Resolver

	// 列名 → FieldIndex 链映射缓存（按模型类型），colIdxMu 互斥。
	colIdxMu sync.Mutex
	colIdx   map[reflect.Type]map[string][]int
}

// preloadSpec 一层 Preload 的定制（conds/callbacks 仅作用于首层子查询，
// 嵌套层以空 spec 递归，与 gorm 语义一致）。
type preloadSpec struct {
	conds     []any
	callbacks []func(contracts.Query) contracts.Query
}

// keyedParent 键值非零、参与本轮 IN 查询的父行及其键值元组。
type keyedParent struct {
	row  preloadRow
	vals []any
}

// resolver 返回生效的关联解析器。
func (e *Engine) resolver() Resolver {
	if e.Resolver != nil {
		return e.Resolver
	}
	return &DefaultResolver{Meta: e.Meta}
}

// Preload 对 parents（*T / []T / []*T，及对应指针形态）按 path 执行关联
// 预加载。path 为点号分隔的字段路径（如 "Orders.Items"）。conds 组成首层
// 子查询的 Where 条件（conds[0] 为条件，其余为参数）；callbacks 在子查询
// 构造后、Find 前依次应用（可排序/分页/追加条件）；many2many 时两者作用
// 于子表查询（第二跳）。
func (e *Engine) Preload(parents any, path string, conds []any, callbacks []func(contracts.Query) contracts.Query) error {
	if e == nil || e.Meta == nil || e.NewQuery == nil {
		return fmt.Errorf("%w: preload.Engine 未配置 Meta/NewQuery", contracts.ErrUnsupported)
	}
	rows, elemType := collectPreloadRows(parents)
	if len(rows) == 0 || elemType == nil {
		return nil
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	return e.preloadLevel(rows, elemType, strings.Split(path, "."), preloadSpec{conds: conds, callbacks: callbacks})
}

// preloadLevel 对一层父行执行单段预加载并在剩余段上递归（契约 1/2/3/4/5）。
func (e *Engine) preloadLevel(rows []preloadRow, elemType reflect.Type, segs []string, sp preloadSpec) error {
	seg := segs[0]
	field, ok := elemType.FieldByName(seg)
	if !ok || field.Anonymous {
		return fmt.Errorf("%w: Preload 字段 %q 在模型 %s 上不存在", contracts.ErrUnsupported, seg, elemType.Name())
	}
	_, isSlice, isRel := relationElemType(field.Type)
	if !isRel {
		return fmt.Errorf("%w: Preload 字段 %q 类型 %s 不是关联类型（需 struct/指针/切片）", contracts.ErrUnsupported, seg, field.Type)
	}
	rel, err := e.resolver().Resolve(elemType, seg, isSlice)
	if err != nil {
		return err
	}

	if rel.Kind == Many2Many {
		return e.preloadMany2Many(rows, elemType, seg, segs, isSlice, rel, sp)
	}

	// 键角色按方向归位：查询键列（IN，恒在子表/被查询表）；父行取值列与
	// 子行分组列取对侧（has 系：父 RefCols/子 FKCols；belongs-to：父 FKCols/子 RefCols）
	queryCols, parentCols, childCols := rel.FKCols, rel.RefCols, rel.FKCols
	if rel.Kind == BelongsTo {
		queryCols, parentCols, childCols = rel.RefCols, rel.FKCols, rel.RefCols
	}

	// 收集父行引用键值：任一列零值/不可定位即跳过该行（契约 4）；全部为零
	// 值时不发起查询（契约 5 前半，零值行关联字段保持零值）
	kps := e.collectParentKeys(rows, elemType, parentCols)
	if len(kps) == 0 {
		return nil
	}

	childTable, err := e.Meta.TableName(rel.ChildType)
	if err != nil {
		return fmt.Errorf("preload: 解析子模型 %s 表名失败: %w", rel.ChildType.Name(), err)
	}
	// 批量 IN 子查询（契约 1），conds/callbacks 作用于本层子查询（契约 3）
	children, err := e.loadChildren(childTable, queryCols, keyedVals(kps), rel, sp)
	if err != nil {
		return err
	}

	// 分组回填：无子行 has-many 回非 nil 空切片、has-one/belongs-to 保持零值（契约 5）
	childIdx := e.columnIndex(rel.ChildType)
	groups := make(map[string][]reflect.Value)
	for i := 0; i < children.Len(); i++ {
		cv := children.Index(i)
		key := joinKeyOf(cv, childCols, childIdx)
		groups[key] = append(groups[key], cv)
	}
	for _, kp := range kps {
		fv, ferr := relationFieldValue(kp.row, seg)
		if ferr != nil {
			return ferr
		}
		group := groups[preloadJoinKey(kp.vals)]
		if isSlice {
			setPreloadSlice(fv, group)
		} else {
			setPreloadSingle(fv, group)
		}
	}
	return e.preloadNested(kps, seg, segs)
}

// preloadMany2Many 两跳预加载（契约 7）：第一跳查中间表映射行（joinFK IN
// 父键集），第二跳以引用键集 IN 查子表并按中间表映射回填（不错位）；引擎
// 对中间表只读（契约 9）。
func (e *Engine) preloadMany2Many(rows []preloadRow, elemType reflect.Type, seg string, segs []string, isSlice bool, rel *Relation, sp preloadSpec) error {
	// 父行键 = RefCols（父表侧连接键，即 joinForeignKey 映射列）
	kps := e.collectParentKeys(rows, elemType, rel.RefCols)
	if len(kps) == 0 {
		return nil // 全零值/无行：不发起任何查询（契约 4/5）
	}
	mappings, err := e.loadJoinMappings(rel, keyedVals(kps))
	if err != nil {
		return err
	}
	// 映射行 → joinFK 值键 → joinRef 值键集合；引用键去重供第二跳 IN
	joinMap := make(map[string]map[string]struct{}, len(mappings))
	seenRef := make(map[string]struct{}, len(mappings))
	refTuples := make([][]any, 0, len(mappings))
	for _, row := range mappings {
		fkVals, merr := mapTuple(row, rel.JoinFKCols)
		if merr != nil {
			return merr
		}
		refVals, merr := mapTuple(row, rel.JoinRefCols)
		if merr != nil {
			return merr
		}
		fk, rf := preloadJoinKey(fkVals), preloadJoinKey(refVals)
		set, ok := joinMap[fk]
		if !ok {
			set = make(map[string]struct{})
			joinMap[fk] = set
		}
		set[rf] = struct{}{}
		if _, dup := seenRef[rf]; !dup {
			seenRef[rf] = struct{}{}
			refTuples = append(refTuples, refVals)
		}
	}

	// 第二跳：子表查询，conds/callbacks/polymorphic 均作用于该跳（契约 3/7/8）
	var children reflect.Value
	if len(refTuples) > 0 {
		childTable, terr := e.Meta.TableName(rel.ChildType)
		if terr != nil {
			return fmt.Errorf("preload: 解析子模型 %s 表名失败: %w", rel.ChildType.Name(), terr)
		}
		if children, err = e.loadChildren(childTable, rel.FKCols, refTuples, rel, sp); err != nil {
			return err
		}
	}

	// 按中间表映射回填：子行按键命中该父行映射到的引用键集合，查询序不乱
	childIdx := e.columnIndex(rel.ChildType)
	type keyedChild struct {
		key string
		val reflect.Value
	}
	kids := make([]keyedChild, 0, children.Len())
	for i := 0; i < children.Len(); i++ {
		cv := children.Index(i)
		kids = append(kids, keyedChild{key: joinKeyOf(cv, rel.FKCols, childIdx), val: cv})
	}
	for _, kp := range kps {
		fv, ferr := relationFieldValue(kp.row, seg)
		if ferr != nil {
			return ferr
		}
		var group []reflect.Value
		if refs := joinMap[preloadJoinKey(kp.vals)]; refs != nil {
			for _, k := range kids {
				if _, hit := refs[k.key]; hit {
					group = append(group, k.val)
				}
			}
		}
		if isSlice {
			setPreloadSlice(fv, group)
		} else {
			setPreloadSingle(fv, group)
		}
	}
	return e.preloadNested(kps, seg, segs)
}

// loadChildren 构造并执行子表查询：单列外键 `col IN ?`（contracts 层 IN
// 切片展开语义）；复合外键手写元组 IN 占位符并按 compositeBatchSize 分批
// （契约 1），各批结果按序合并。polymorphic 类型列条件、conds 与 callbacks
// 在 Find 前按序应用（契约 3/8）。
func (e *Engine) loadChildren(table string, queryCols []string, parentTuples [][]any, rel *Relation, sp preloadSpec) (reflect.Value, error) {
	sliceType := reflect.SliceOf(rel.ChildType)
	children := reflect.MakeSlice(sliceType, 0, 0)

	if len(queryCols) == 1 {
		ids := make([]any, 0, len(parentTuples))
		for _, t := range parentTuples {
			ids = append(ids, t[0])
		}
		q := e.NewQuery().Table(table).Where(queryCols[0]+" IN ?", ids)
		q, err := e.applyChildTail(q, rel, sp)
		if err != nil {
			return children, err
		}
		slicePtr := reflect.New(sliceType)
		if err := q.Find(slicePtr.Interface()); err != nil {
			return children, err
		}
		return slicePtr.Elem(), nil
	}

	for start := 0; start < len(parentTuples); start += compositeBatchSize {
		end := start + compositeBatchSize
		if end > len(parentTuples) {
			end = len(parentTuples)
		}
		batch := parentTuples[start:end]
		ph := "(" + strings.TrimSuffix(strings.Repeat("?, ", len(queryCols)), ", ") + ")"
		phs := strings.TrimSuffix(strings.Repeat(ph+", ", len(batch)), ", ")
		args := make([]any, 0, len(batch)*len(queryCols))
		for _, t := range batch {
			args = append(args, t...)
		}
		q := e.NewQuery().Table(table).Where("("+strings.Join(queryCols, ", ")+") IN ("+phs+")", args...)
		q, err := e.applyChildTail(q, rel, sp)
		if err != nil {
			return children, err
		}
		slicePtr := reflect.New(sliceType)
		if err := q.Find(slicePtr.Interface()); err != nil {
			return children, err
		}
		children = reflect.AppendSlice(children, slicePtr.Elem())
	}
	return children, nil
}

// loadJoinMappings many2many 第一跳：查中间表 (joinFK..., joinRef...) 映射行
// （ScanMap，键为列名）。中间表模型无需业务定义，引擎直接构造表级子查询且
// 只读（契约 7/9）；joinFK 复合时同样元组 IN 分批。
func (e *Engine) loadJoinMappings(rel *Relation, parentTuples [][]any) ([]map[string]any, error) {
	var mappings []map[string]any
	scan := func(q contracts.Query) error {
		return q.ScanMap(&mappings)
	}
	first, rest := selectColumns(rel.JoinFKCols, rel.JoinRefCols)
	if len(rel.JoinFKCols) == 1 {
		ids := make([]any, 0, len(parentTuples))
		for _, t := range parentTuples {
			ids = append(ids, t[0])
		}
		q := e.NewQuery().Table(rel.Many2Many).Select(first, rest...).Where(rel.JoinFKCols[0]+" IN ?", ids)
		return mappings, scan(q)
	}
	for start := 0; start < len(parentTuples); start += compositeBatchSize {
		end := start + compositeBatchSize
		if end > len(parentTuples) {
			end = len(parentTuples)
		}
		batch := parentTuples[start:end]
		ph := "(" + strings.TrimSuffix(strings.Repeat("?, ", len(rel.JoinFKCols)), ", ") + ")"
		phs := strings.TrimSuffix(strings.Repeat(ph+", ", len(batch)), ", ")
		args := make([]any, 0, len(batch)*len(rel.JoinFKCols))
		for _, t := range batch {
			args = append(args, t...)
		}
		q := e.NewQuery().Table(rel.Many2Many).Select(first, rest...).
			Where("("+strings.Join(rel.JoinFKCols, ", ")+") IN ("+phs+")", args...)
		if err := scan(q); err != nil {
			return nil, err
		}
	}
	return mappings, nil
}

// selectColumns 组装 Select 的字符串列参数（contracts.Select(query any,
// args ...any) 的 string 变参形态，gorm/xorm 双驱动均支持）。
func selectColumns(groups ...[]string) (string, []any) {
	var all []string
	for _, g := range groups {
		all = append(all, g...)
	}
	rest := make([]any, 0, len(all))
	for _, c := range all[1:] {
		rest = append(rest, c)
	}
	return all[0], rest
}

// applyChildTail 在子查询上依次应用 polymorphic 类型列条件（契约 8）、conds
// （契约 3）与 callbacks（子查询构造后、Find 前依次应用）。
func (e *Engine) applyChildTail(q contracts.Query, rel *Relation, sp preloadSpec) (contracts.Query, error) {
	if rel.PolyColumn != "" {
		q = q.Where(rel.PolyColumn+" = ?", rel.PolyValue)
	}
	if len(sp.conds) > 0 {
		q = q.Where(sp.conds[0], sp.conds[1:]...)
	}
	for _, cb := range sp.callbacks {
		next := cb(q)
		if next == nil {
			return nil, fmt.Errorf("%w: Preload 回调返回了 nil Query", contracts.ErrUnsupported)
		}
		q = next
	}
	return q, nil
}

// preloadNested 剩余段以本层已回填到父行的子行作为父行递归（契约 2）。
// 必须从字段值重新收集而非复用装载数组：has-many 值切片字段（[]T）回填时
// 元素被复制，装载数组里的原件与调用方持有的副本不是同一结构体，直接递归
// 会写丢。嵌套层 conds/callbacks 不生效（与 gorm 语义一致）。
func (e *Engine) preloadNested(kps []keyedParent, seg string, segs []string) error {
	if len(segs) <= 1 {
		return nil
	}
	var next []preloadRow
	var nextType reflect.Type
	for _, kp := range kps {
		fv, err := relationFieldValue(kp.row, seg)
		if err != nil {
			return err
		}
		sub, st := collectPreloadRows(fv.Addr().Interface())
		next = append(next, sub...)
		if st != nil {
			nextType = st
		}
	}
	if len(next) > 0 {
		return e.preloadLevel(next, nextType, segs[1:], preloadSpec{})
	}
	return nil
}

// collectParentKeys 收集父行键值元组：任一键列不可定位或为零值即跳过该行
// （契约 4——其关联字段保持零值、不参与 IN 查询）。
func (e *Engine) collectParentKeys(rows []preloadRow, elemType reflect.Type, cols []string) []keyedParent {
	idx := e.columnIndex(elemType)
	out := make([]keyedParent, 0, len(rows))
	for _, row := range rows {
		rv := reflect.Indirect(reflect.ValueOf(row.ptr))
		vals := make([]any, 0, len(cols))
		zero := false
		for _, col := range cols {
			fi, ok := idx[col]
			if !ok {
				zero = true
				break
			}
			fv := valueByIndex(rv, fi)
			if !fv.IsValid() || fv.IsZero() {
				zero = true
				break
			}
			vals = append(vals, fv.Interface())
		}
		if zero {
			continue
		}
		out = append(out, keyedParent{row: row, vals: vals})
	}
	return out
}

// columnIndex 取（并缓存）模型类型"列名 → FieldIndex 链"映射。
func (e *Engine) columnIndex(t reflect.Type) map[string][]int {
	e.colIdxMu.Lock()
	defer e.colIdxMu.Unlock()
	if idx, ok := e.colIdx[t]; ok {
		return idx
	}
	idx := columnFieldIndexes(t, e.Meta)
	if e.colIdx == nil {
		e.colIdx = make(map[reflect.Type]map[string][]int)
	}
	e.colIdx[t] = idx
	return idx
}

// keyedVals 提取父行键值元组列表（保持父行顺序）。
func keyedVals(kps []keyedParent) [][]any {
	tuples := make([][]any, 0, len(kps))
	for _, kp := range kps {
		tuples = append(tuples, kp.vals)
	}
	return tuples
}

// joinKeyOf 取行上多列值并组合为分组键（列不可定位以 nil 占位）。
func joinKeyOf(rowVal reflect.Value, cols []string, idx map[string][]int) string {
	dv := rowVal
	if dv.Kind() == reflect.Ptr {
		dv = dv.Elem()
	}
	vals := make([]any, 0, len(cols))
	for _, col := range cols {
		fi, ok := idx[col]
		if !ok {
			vals = append(vals, nil)
			continue
		}
		fv := valueByIndex(dv, fi)
		if !fv.IsValid() {
			vals = append(vals, nil)
			continue
		}
		vals = append(vals, fv.Interface())
	}
	return preloadJoinKey(vals)
}

// mapTuple 从中间表映射行（ScanMap 行）提取多列值；键精确未命中时大小写
// 不敏感兜底（防驱动列名归一化差异），缺列报 ErrUnsupported。
func mapTuple(row map[string]any, cols []string) ([]any, error) {
	vals := make([]any, 0, len(cols))
	for _, col := range cols {
		v, ok := mapValue(row, col)
		if !ok {
			return nil, fmt.Errorf("%w: many2many 中间表查询结果缺少列 %q", contracts.ErrUnsupported, col)
		}
		vals = append(vals, v)
	}
	return vals, nil
}

// mapValue 映射行取列值：精确键优先，失败时大小写不敏感回退。
func mapValue(row map[string]any, col string) (any, bool) {
	if v, ok := row[col]; ok {
		return v, true
	}
	for k, v := range row {
		if strings.EqualFold(k, col) {
			return v, true
		}
	}
	return nil, false
}
