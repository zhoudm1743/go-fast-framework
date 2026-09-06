package preload

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// ── 反射工具（自 gofast-xorm/query_preload.go 抽提，去驱动化）─────────
//
// 本文件只依赖标准库（reflect/fmt/strings），不含任何 ORM/SQL 驱动语义。

// preloadRow 一行可回填关联字段的父行（可寻址 struct 指针）。
type preloadRow struct {
	ptr any
}

// collectPreloadRows 将 parents 展开为可寻址父行列表，并返回元素结构体类型。
// 支持 *T、[]T、[]*T、*[]T、*[]*T；nil 指针、nil 元素与不可寻址元素跳过。
// 非结构体 dest 返回空结果，由调用方跳过预加载。
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
// group 为子行 reflect.Value（元素为 T/*T 两种来源）。空组时回填非 nil 空切片
// （契约 5）。先在局部组装完整切片再一次性写回字段：MakeSlice 返回的 header
// 不可写穿，中途 Append 重赋值只更新局部变量，字段持有的仍是 len 0 的旧
// header，必须末尾统一 Set。
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

// setPreloadSingle 将子行组首行回填到 has-one/belongs-to 字段（T / *T）；
// 无子行保持零值（契约 5）。
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

// preloadJoinKey 拼接键值列表为分组键（\x1f 不可见分隔符组合多列，
// 与缓存键片段约定一致）。
func preloadJoinKey(vals []any) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = fmt.Sprint(v)
	}
	return strings.Join(parts, "\x1f")
}

// valueByIndex 沿 FieldIndex 链定位字段值（逐级解引用并校验指针/结构有效性，
// 天然支持嵌入 extends 路径；不可达时返回零 Value）。
func valueByIndex(rv reflect.Value, index []int) reflect.Value {
	cur := rv
	for _, idx := range index {
		if cur.Kind() == reflect.Ptr {
			if cur.IsNil() {
				return reflect.Value{}
			}
			cur = cur.Elem()
		}
		if cur.Kind() != reflect.Struct {
			return reflect.Value{}
		}
		cur = cur.Field(idx)
	}
	return cur
}

// relationFieldValue 按字段名定位父行上可回填的关联字段：先 FieldByName
// 取得字段（含嵌入提升路径）的 FieldIndex 链，再指针安全寻址；字段不可见
// 或不可写时返回 ErrUnsupported 包装错误。
func relationFieldValue(row preloadRow, fieldName string) (reflect.Value, error) {
	rv := reflect.ValueOf(row.ptr)
	if rv.Kind() != reflect.Ptr || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("%w: Preload 父行不是可寻址 struct 指针，无法回填字段 %q", contracts.ErrUnsupported, fieldName)
	}
	fv := rv.Elem().FieldByName(fieldName)
	if !fv.IsValid() || !fv.CanSet() {
		return fv, fmt.Errorf("%w: Preload 字段 %q 不可回填（需为导出且可寻址的关联字段）", contracts.ErrUnsupported, fieldName)
	}
	return fv, nil
}

// columnFieldIndexes 建立"列名 → FieldIndex 链"映射：递归展开匿名嵌入
// struct，每个叶子字段经 MetaAdapter.ColumnOfField 解析列名（先按顶层模型
// 类型解析，再按字段所属嵌入类型解析兜底，兼容两类驱动实现）。同名列以
// 先出现者为准。
func columnFieldIndexes(t reflect.Type, meta MetaAdapter) map[string][]int {
	out := make(map[string][]int)
	var walk func(rt reflect.Type, prefix []int, owner reflect.Type)
	walk = func(rt reflect.Type, prefix []int, owner reflect.Type) {
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			idx := append(append([]int(nil), prefix...), i)
			if f.Anonymous {
				ft := f.Type
				for ft.Kind() == reflect.Ptr {
					ft = ft.Elem()
				}
				if ft.Kind() == reflect.Struct {
					walk(ft, idx, ft)
					continue
				}
			}
			if f.PkgPath != "" {
				continue // 未导出字段不可读写
			}
			col, ok := meta.ColumnOfField(t, f.Name)
			if !ok {
				col, ok = meta.ColumnOfField(owner, f.Name)
			}
			if !ok {
				continue
			}
			if _, exists := out[col]; exists {
				continue
			}
			out[col] = idx
		}
	}
	walk(t, nil, t)
	return out
}
