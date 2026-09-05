package xormdriver

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/zhoudm1743/go-fast-framework/contracts"

	"xorm.io/builder"
	"xorm.io/xorm/schemas"
)

// ── 关联预加载 ───────────────────────────────────────────────────────
//
// xorm 没有 gorm 的关联元数据（association tag），无法原生按 gorm 语义把关联
// 模型装载进结构体字段，这里以"约定 + 反射回填"实现 Preload：
//
//   - 关联字段：导出、非匿名字段，tag 必须为 xorm:"-"（xorm 对 struct/slice
//     字段无法映射为列，不打忽略标记 TableInfo/Sync2 会解析失败），类型为
//     struct / *struct / []struct / []*struct / *[]struct；
//   - 外键解析（resolvePreloadKeys）：关联字段的 gorm tag
//     （foreignKey:字段[,字段];references:字段[,字段]，逗号分隔支持复合键）
//     优先；无 tag 时回退多列约定——父表每个主键列对应子表外键列
//     <父表名>_<主键列>（单列主键 id 即 <父表名>_id）。列逐一校验存在，
//     缺失返回 ErrUnsupported 并提示改用 Joins；
//   - 加载方式：按父行引用键值对子表做一次批量 IN 查询（非 N+1；复合键为
//     元组 IN），再按外键值分组回填；嵌套路径（"Orders.Items"）以本层回填
//     到字段的子行作为下一层父行递归处理；
//   - 定制：args 中的 func(contracts.Query) contracts.Query 项提取为子查询
//     回调（gorm 回调的驱动无关版本，可排序/分页/追加条件），其余项作为子
//     查询 Where 条件（string/builder.Cond/map[string]any），两者可共存；
//   - 限制：Pluck/Scan 等无结构体 dest 的终结方法不触发预加载；gorm 的
//     joins（多对多中间表）不支持，复杂场景请用 Joins 或 Raw 自行装配。

// preloadSpec 一条 Preload 声明（链上记录，终结时执行）。
type preloadSpec struct {
	path      string                                  // 关联字段路径，如 "Profile"、"Orders.Items"
	conds     []any                                   // 子查询 Where 条件（首个为条件，其余参数）
	callbacks []func(contracts.Query) contracts.Query // 子查询定制回调
}

// Preload 声明关联预加载。query 为字段路径（点号嵌套）。args 中
// func(contracts.Query) contracts.Query 类型的项提取为子查询定制回调
// （在子查询构造后、执行前应用，可排序/分页/追加条件），其余项组成首层子
// 查询的 Where 条件（与 gorm 语义一致）；条件类型在链上校验，非法即记入
// 链上错误，由终结方法返回 ErrUnsupported 包装。预加载在父查询成功装载行
// 后执行（Find/First/Last/Take/FindInBatches；缓存命中路径同样执行）。
func (q *XormQuery) Preload(query string, args ...any) contracts.Query {
	spec := preloadSpec{path: query}
	for _, a := range args {
		if cb, ok := a.(func(contracts.Query) contracts.Query); ok {
			spec.callbacks = append(spec.callbacks, cb)
			continue
		}
		spec.conds = append(spec.conds, a)
	}
	if len(spec.conds) > 0 && !validCondType(spec.conds[0]) {
		return q.setErr(fmt.Errorf("%w: Preload 条件仅支持 string/builder.Cond/map[string]any，收到 %T", contracts.ErrUnsupported, spec.conds[0]))
	}
	return q.wrap(func(c *XormQuery) {
		c.preloads = append(c.preloads, spec)
	})
}

// runPreloads 在父查询装载行成功后执行全部已声明的预加载。
// dest 为 Find/getOne/FindInBatches 装载后的结果（*T / *[]T / *[]*T 等）。
// 各声明独立按序执行：字段解析/外键校验失败经 ErrUnsupported 包装，子查询
// 错误原样透传，均由终结方法经 q.done 归一为框架 Sentinel Error。
func (q *XormQuery) runPreloads(dest any) error {
	if len(q.preloads) == 0 || dest == nil {
		return nil
	}
	for _, spec := range q.preloads {
		if err := q.preloadPath(dest, strings.Split(spec.path, "."), spec); err != nil {
			return err
		}
	}
	return nil
}

// preloadPath 沿字段路径递归预加载：处理首段，再以本层回填到字段的子行作为
// 父行继续处理剩余段。spec 的 conds/callbacks 仅作用于首层子查询（与 gorm
// 语义一致），嵌套层以空 spec 递归。
func (q *XormQuery) preloadPath(parents any, segs []string, spec preloadSpec) error {
	// 展开 dest 为可寻址父行列表与元素结构体类型；无行/非结构体 dest 直接跳过
	rows, elemType := collectPreloadRows(parents)
	if len(rows) == 0 || elemType == nil {
		return nil
	}
	return q.preloadLevel(rows, elemType, segs, spec)
}

// preloadLevel 对一层父行执行单段预加载并在剩余段上递归。
func (q *XormQuery) preloadLevel(rows []preloadRow, elemType reflect.Type, segs []string, spec preloadSpec) error {
	seg := segs[0]
	field, ok := elemType.FieldByName(seg)
	if !ok || field.Anonymous {
		return fmt.Errorf("%w: Preload 字段 %q 在模型 %s 上不存在", contracts.ErrUnsupported, seg, elemType.Name())
	}
	// 关联字段必须带 xorm:"-"：xorm 对 struct/slice 字段无法映射为列，
	// 未标记忽略的模型 TableInfo 解析即失败，不存在"未标记但可预加载"的字段
	if tag := strings.TrimSpace(field.Tag.Get("xorm")); tag != "-" {
		return fmt.Errorf("%w: Preload 字段 %q 不是关联字段（关联字段需 xorm:\"-\" 标记）", contracts.ErrUnsupported, seg)
	}
	childType, isSlice, ok := relationElemType(field.Type)
	if !ok {
		return fmt.Errorf("%w: Preload 字段 %q 类型 %s 不是关联类型（需 struct/指针/切片）", contracts.ErrUnsupported, seg, field.Type)
	}

	// 父表元数据：表名参与外键列约定，主键（或 gorm references）决定父行键
	parentTI, err := q.engine.TableInfo(reflect.New(elemType).Interface())
	if err != nil {
		return fmt.Errorf("xorm: Preload 父模型 %s 解析失败: %w", elemType.Name(), err)
	}
	childTI, err := q.engine.TableInfo(reflect.New(childType).Interface())
	if err != nil {
		return fmt.Errorf("xorm: Preload 子模型 %s 解析失败: %w", childType.Name(), err)
	}

	// 外键解析：gorm tag 优先，缺省多列约定 <父表名>_<主键列>（含存在性校验）
	fkCols, refCols, err := resolvePreloadKeys(field, parentTI, childTI)
	if err != nil {
		return err
	}
	refFields := make([]string, len(refCols))
	for i, c := range refCols {
		refFields[i] = parentTI.GetColumn(c).FieldName
	}
	fkFields := make([]string, len(fkCols))
	for i, c := range fkCols {
		fkFields[i] = childTI.GetColumn(c).FieldName
	}

	// 收集父行引用键值（任一引用键零值即跳过该行，其关联字段保持零值）
	ids := make([]any, 0, len(rows)*len(refFields))
	indexed := make([]preloadRow, 0, len(rows))
	for _, row := range rows {
		rv := reflect.Indirect(reflect.ValueOf(row.ptr))
		vals := make([]any, 0, len(refFields))
		zero := false
		for _, f := range refFields {
			fv := rv.FieldByName(f)
			if !fv.IsValid() || fv.IsZero() {
				zero = true
				break
			}
			vals = append(vals, fv.Interface())
		}
		if zero {
			continue
		}
		ids = append(ids, vals...)
		indexed = append(indexed, row)
	}

	// 批量 IN 查询子行（继承 schema/ctx/事务会话/查询缓存配置；不继承父链条件）；
	// 单列外键 builder.In，复合外键元组 IN；spec 条件与回调在 Find 前应用。
	var children reflect.Value
	if len(indexed) > 0 {
		child := &XormQuery{
			engine:   q.engine,
			tx:       q.tx,
			schema:   q.schema,
			ctx:      q.ctx,
			qc:       q.qc,
			cacheCfg: q.cacheCfg,
		}
		child = child.Table(q.schemaTable(childTI.Name)).(*XormQuery)
		if len(fkCols) == 1 {
			child = child.Where(builder.In(fkCols[0], ids...)).(*XormQuery)
		} else {
			ph := "(" + strings.TrimSuffix(strings.Repeat("?, ", len(fkCols)), ", ") + ")"
			phs := strings.TrimSuffix(strings.Repeat(ph+", ", len(indexed)), ", ")
			child = child.Where("("+strings.Join(fkCols, ", ")+") IN ("+phs+")", ids...).(*XormQuery)
		}
		if len(spec.conds) > 0 {
			child = child.Where(spec.conds[0], spec.conds[1:]...).(*XormQuery)
		}
		for _, cb := range spec.callbacks {
			next, ok := cb(child).(*XormQuery)
			if !ok || next == nil {
				return fmt.Errorf("%w: Preload 回调返回值非法（需返回 contracts.Query 链）", contracts.ErrUnsupported)
			}
			child = next
		}
		slicePtr := reflect.New(reflect.SliceOf(childType))
		if err := child.Find(slicePtr.Interface()); err != nil {
			return err
		}
		children = slicePtr.Elem()
	}

	// 按外键值分组回填父行
	if len(indexed) > 0 && children.IsValid() && children.Len() > 0 {
		groups := make(map[string][]reflect.Value)
		for i := 0; i < children.Len(); i++ {
			cv := children.Index(i)
			dv := cv
			if dv.Kind() == reflect.Ptr {
				dv = dv.Elem()
			}
			vals := make([]any, 0, len(fkFields))
			for _, f := range fkFields {
				vals = append(vals, dv.FieldByName(f).Interface())
			}
			key := preloadJoinKey(vals)
			groups[key] = append(groups[key], cv)
		}
		for _, row := range indexed {
			rv := reflect.Indirect(reflect.ValueOf(row.ptr))
			vals := make([]any, 0, len(refFields))
			for _, f := range refFields {
				vals = append(vals, rv.FieldByName(f).Interface())
			}
			fv := reflect.ValueOf(row.ptr).Elem().FieldByName(seg)
			group := groups[preloadJoinKey(vals)]
			if isSlice {
				setPreloadSlice(fv, group)
			} else {
				setPreloadSingle(fv, group)
			}
		}
	} else if isSlice {
		// 无子行但父行引用键非零：has-many 回填空切片，语义明确（gorm 同）
		for _, row := range indexed {
			setPreloadSlice(reflect.ValueOf(row.ptr).Elem().FieldByName(seg), nil)
		}
	}

	// 剩余段以本层已回填到父行的子行作为父行递归。必须从字段值收集父行
	// 而非复用装载数组：has-many 值切片字段（[]T）回填时元素被复制，
	// 装载数组里的原件与调用方持有的副本不是同一结构体，直接递归会写丢。
	if len(segs) > 1 {
		var next []preloadRow
		var nextType reflect.Type
		for _, row := range indexed {
			fv := reflect.ValueOf(row.ptr).Elem().FieldByName(seg)
			sub, st := collectPreloadRows(fv.Addr().Interface())
			next = append(next, sub...)
			if st != nil {
				nextType = st
			}
		}
		if len(next) > 0 {
			return q.preloadLevel(next, nextType, segs[1:], preloadSpec{})
		}
	}
	return nil
}

// preloadRow 一行可回填关联字段的父行（可寻址 struct 指针）。
type preloadRow struct {
	ptr any
}

// ── 外键解析 ─────────────────────────────────────────────────────────

// resolvePreloadKeys 解析父键列（refCols）与子表外键列（fkCols），均为数据库列名。
// 关联字段的 gorm tag（foreignKey/references，字段名逗号分隔、复合键支持）优先：
// foreignKey 值为子模型字段名，references 值为父模型字段名，缺省 references
// 回退父表主键、缺省 foreignKey 回退多列约定。无 tag 时整体回退多列约定：
// 父表每个主键列对应子表 <父表名>_<主键列> 外键列（单列主键 id 即 <父表名>_id）。
// 引用/外键列逐一校验存在，失败返回 ErrUnsupported 包装错误。
func resolvePreloadKeys(field reflect.StructField, parentTI, childTI *schemas.Table) (fkCols, refCols []string, err error) {
	fkNames, refNames, hasTag := gormAssocKeys(field.Tag.Get("gorm"))
	if hasTag {
		if refNames != "" {
			if refCols, err = fieldNamesToColumns(parentTI, refNames); err != nil {
				return nil, nil, fmt.Errorf("%w: Preload 字段 %q gorm references 解析失败: %v", contracts.ErrUnsupported, field.Name, err)
			}
		} else {
			refCols = append([]string(nil), parentTI.PrimaryKeys...)
		}
		if fkNames != "" {
			if fkCols, err = fieldNamesToColumns(childTI, fkNames); err != nil {
				return nil, nil, fmt.Errorf("%w: Preload 字段 %q gorm foreignKey 解析失败: %v", contracts.ErrUnsupported, field.Name, err)
			}
		} else {
			for _, rc := range refCols {
				fkCols = append(fkCols, parentTI.Name+"_"+rc)
			}
		}
	} else {
		refCols = append([]string(nil), parentTI.PrimaryKeys...)
		for _, rc := range refCols {
			fkCols = append(fkCols, parentTI.Name+"_"+rc)
		}
	}
	if len(refCols) == 0 || len(fkCols) == 0 {
		return nil, nil, fmt.Errorf("%w: Preload 字段 %q 父表 %q 无主键可关联，请改用 Joins", contracts.ErrUnsupported, field.Name, parentTI.Name)
	}
	if len(fkCols) != len(refCols) {
		return nil, nil, fmt.Errorf("%w: Preload 字段 %q 外键列数（%d）与引用键列数（%d）不一致", contracts.ErrUnsupported, field.Name, len(fkCols), len(refCols))
	}
	for i, c := range fkCols {
		if childTI.GetColumn(c) == nil {
			return nil, nil, fmt.Errorf("%w: Preload 字段 %q 子表 %q 缺少外键列 %q，请改用 Joins", contracts.ErrUnsupported, field.Name, childTI.Name, c)
		}
		if parentTI.GetColumn(refCols[i]) == nil {
			return nil, nil, fmt.Errorf("%w: Preload 字段 %q 父表 %q 缺少引用列 %q", contracts.ErrUnsupported, field.Name, parentTI.Name, refCols[i])
		}
	}
	return fkCols, refCols, nil
}

// gormAssocKeys 解析 gorm association tag 中的 foreignKey/references 字段名
// 列表。两个 key 均缺省时 ok=false（调用方回退多列约定）。
func gormAssocKeys(tag string) (foreignKey, references string, ok bool) {
	for _, part := range strings.Split(tag, ";") {
		part = strings.TrimSpace(part)
		switch {
		case strings.HasPrefix(part, "foreignKey:"):
			foreignKey = strings.TrimSpace(strings.TrimPrefix(part, "foreignKey:"))
			ok = true
		case strings.HasPrefix(part, "references:"):
			references = strings.TrimSpace(strings.TrimPrefix(part, "references:"))
			ok = true
		}
	}
	return foreignKey, references, ok
}

// fieldNamesToColumns 将结构体字段名列表（逗号分隔）映射为数据库列名；
// 字段不存在（或非列字段）时报错，由调用方包装 ErrUnsupported。
func fieldNamesToColumns(ti *schemas.Table, names string) ([]string, error) {
	cols := make([]string, 0, 2)
	for _, n := range strings.Split(names, ",") {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		col := columnOfFieldName(ti, n)
		if col == nil {
			return nil, fmt.Errorf("字段 %q 在表 %q 中不存在", n, ti.Name)
		}
		cols = append(cols, col.Name)
	}
	return cols, nil
}

// columnOfFieldName 按结构体字段名在表元数据中定位列。
func columnOfFieldName(ti *schemas.Table, fieldName string) *schemas.Column {
	for _, c := range ti.Columns() {
		if c.FieldName == fieldName {
			return c
		}
	}
	return nil
}

// preloadJoinKey 拼接键值列表为分组键（\x1f 分隔，与缓存键片段约定一致）。
func preloadJoinKey(vals []any) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = fmt.Sprint(v)
	}
	return strings.Join(parts, "\x1f")
}

// collectPreloadRows 将 dest 展开为可寻址父行列表，并返回元素结构体类型。
// 支持 *T、*[]T、*[]*T、[]T、[]*T；nil 指针、nil 元素与不可寻址元素跳过。
// 非结构体 dest（nil）返回空结果，由调用方跳过预加载。
func collectPreloadRows(dest any) ([]preloadRow, reflect.Type) {
	rv := reflect.ValueOf(dest)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return nil, nil
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		if rv.Len() == 0 {
			return nil, nil
		}
		elemType := rv.Type().Elem()
		for elemType.Kind() == reflect.Ptr {
			elemType = elemType.Elem()
		}
		if elemType.Kind() != reflect.Struct {
			return nil, nil
		}
		rows := make([]preloadRow, 0, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			e := rv.Index(i)
			switch {
			case e.Kind() == reflect.Ptr:
				if e.IsNil() {
					continue
				}
				rows = append(rows, preloadRow{ptr: e.Interface()})
			case e.CanAddr():
				rows = append(rows, preloadRow{ptr: e.Addr().Interface()})
			}
		}
		return rows, elemType
	case reflect.Struct:
		if !rv.CanAddr() {
			return nil, nil
		}
		return []preloadRow{{ptr: rv.Addr().Interface()}}, rv.Type()
	default:
		return nil, nil
	}
}

// relationElemType 解析关联字段类型：解引用指针/切片得到子元素结构体类型，
// isSlice 表示 has-many（字段含切片）；非关联类型 ok=false。
func relationElemType(t reflect.Type) (childType reflect.Type, isSlice, ok bool) {
	if t.Kind() == reflect.Ptr && t.Elem().Kind() == reflect.Slice {
		t, isSlice = t.Elem(), true
	}
	if t.Kind() == reflect.Slice {
		t, isSlice = t.Elem(), true
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, false, false
	}
	return t, isSlice, true
}

// setPreloadSlice 将子行组回填到 has-many 字段（[]T / []*T / *[]T）。
// group 为子行 reflect.Value（元素为 T/*T 两种来源）。先在局部组装完整切片
// 再一次性写回字段：MakeSlice 返回的 header 不可写穿，中途 Append 重赋值只
// 更新局部变量，字段持有的仍是 len 0 的旧 header，必须末尾统一 Set。
func setPreloadSlice(field reflect.Value, group []reflect.Value) {
	t := field.Type()
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	slice := reflect.MakeSlice(t, 0, len(group))
	elemT := t.Elem()
	for _, g := range group {
		switch {
		case elemT.Kind() == reflect.Ptr && g.Kind() == reflect.Ptr:
			slice = reflect.Append(slice, g)
		case elemT.Kind() == reflect.Ptr:
			slice = reflect.Append(slice, g.Addr())
		case g.Kind() == reflect.Ptr:
			slice = reflect.Append(slice, g.Elem())
		default:
			slice = reflect.Append(slice, g)
		}
	}
	if field.Kind() == reflect.Ptr {
		p := reflect.New(t)
		p.Elem().Set(slice)
		field.Set(p)
	} else {
		field.Set(slice)
	}
}

// setPreloadSingle 将子行组首行回填到 has-one 字段（T / *T）；无子行保持零值。
func setPreloadSingle(field reflect.Value, group []reflect.Value) {
	if len(group) == 0 {
		return
	}
	g := group[0]
	switch {
	case field.Kind() == reflect.Ptr && g.Kind() == reflect.Ptr:
		field.Set(g)
	case field.Kind() == reflect.Ptr:
		field.Set(g.Addr())
	case g.Kind() == reflect.Ptr:
		field.Set(g.Elem())
	default:
		field.Set(g)
	}
}
