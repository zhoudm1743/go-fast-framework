package utils

import (
	"fmt"
	"reflect"
	"strings"
)

// StructUtil 结构体工具集。
var StructUtil = structUtil{}

type structUtil struct{}

// PtrToMap 将结构体中所有非 nil 指针字段收集为 map[string]any。
// key 优先取 json / form / query 标签名，无标签时取字段名的 snake_case 形式。
// 嵌套结构体（值或指针）会递归摊平到同一 map。
//
// 适用于部分更新（patch）场景：仅更新请求中显式传入的字段。
//
// 例：
//
//	type UpdateReq struct {
//		Code *string `json:"code"`
//		Name *string `json:"name"`
//	}
//
//	name := "tom"
//	req := &UpdateReq{Name: &name}
//	updates := utils.StructUtil.PtrToMap(req) // {"name": "tom"}
func (r structUtil) PtrToMap(obj any) map[string]any {
	m := make(map[string]any)
	r.collect(reflect.ValueOf(obj), m, true)
	return m
}

// StructToMap 将结构体中所有非零值字段收集为 map[string]any。
// key 规则与 PtrToMap 一致，嵌套结构体递归摊平。
//
// 与 PtrToMap 的区别：包含非指针字段（只要值非零），
// 适用于将实体完整转成 map（自动跳过零值字段）。
func (r structUtil) StructToMap(obj any) map[string]any {
	m := make(map[string]any)
	r.collect(reflect.ValueOf(obj), m, false)
	return m
}

func (r structUtil) collect(rv reflect.Value, m map[string]any, onlyPtr bool) {
	if !rv.IsValid() {
		return
	}
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return
	}

	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		fv := rv.Field(i)

		if !fv.CanInterface() {
			continue
		}
		if r.ignored(field) {
			continue
		}

		switch fv.Kind() {
		case reflect.Ptr:
			if fv.IsNil() {
				continue
			}
			elem := fv.Elem()
			if elem.Kind() == reflect.Struct {
				r.collect(elem, m, onlyPtr)
			} else if onlyPtr || !elem.IsZero() {
				m[r.key(field)] = elem.Interface()
			}
		case reflect.Struct:
			r.collect(fv, m, onlyPtr)
		default:
			if !onlyPtr && !fv.IsZero() {
				m[r.key(field)] = fv.Interface()
			}
		}
	}
}

// Copy 将 src 的同名字段拷贝到 dst（类型兼容时自动转换）。
// dst 必须是指向结构体的指针，src 可为结构体或结构体指针。
// 仅拷贝导出字段，字段名完全一致时才会赋值。
//
// 适用于实体 ↔ DTO 之间的字段转换。
func (r structUtil) Copy(dst, src any) error {
	dv := reflect.ValueOf(dst)
	if dv.Kind() != reflect.Ptr || dv.IsNil() {
		return fmt.Errorf("目标必须是可修改的结构体指针")
	}
	dv = dv.Elem()
	if dv.Kind() != reflect.Struct {
		return fmt.Errorf("目标必须是结构体")
	}

	sv := reflect.ValueOf(src)
	if sv.Kind() == reflect.Ptr {
		if sv.IsNil() {
			return fmt.Errorf("源为 nil 指针")
		}
		sv = sv.Elem()
	}
	if sv.Kind() != reflect.Struct {
		return fmt.Errorf("源必须是结构体")
	}

	st := sv.Type()
	for i := 0; i < st.NumField(); i++ {
		sf := st.Field(i)
		if sf.PkgPath != "" { // 跳过未导出字段
			continue
		}
		df := dv.FieldByName(sf.Name)
		if !df.IsValid() || !df.CanSet() {
			continue
		}

		sval := sv.Field(i)
		switch {
		case sval.Type().AssignableTo(df.Type()):
			df.Set(sval)
		case sval.Type().ConvertibleTo(df.Type()):
			df.Set(sval.Convert(df.Type()))
		}
	}
	return nil
}

// IsZero 判断值是否为零值（nil 视为零值）。
// 注意：非 nil 指针本身不被视为零值，即使其指向零值。
func (r structUtil) IsZero(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return true
	}
	return rv.IsZero()
}

// ignored 判断字段是否被 json/form/query 标签显式排除（tag 为 "-"）。
func (r structUtil) ignored(field reflect.StructField) bool {
	for _, tagKey := range []string{"json", "form", "query"} {
		tag := field.Tag.Get(tagKey)
		if tag != "" && strings.SplitN(tag, ",", 2)[0] == "-" {
			return true
		}
	}
	return false
}

func (r structUtil) key(field reflect.StructField) string {
	for _, tagKey := range []string{"json", "form", "query"} {
		tag := field.Tag.Get(tagKey)
		if tag == "" {
			continue
		}
		if name := strings.SplitN(tag, ",", 2)[0]; name != "" {
			return name
		}
	}
	return StringUtil.Of(field.Name).Snake().String()
}
