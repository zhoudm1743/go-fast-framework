package base

import (
	"encoding"
	"reflect"
	"strings"
	"time"
)

// ApplyDefaults 遍历结构体字段，对零值字段应用 `default:"xxx"` 标签指定的默认值。
//
// 规则：
//   - 仅当字段为零值时才填充默认值，请求中已传入的值不会被覆盖。
//   - default 为空或 "-" 时视为无默认值。
//   - 嵌套结构体（含指针）会递归处理，nil 指针不会自动分配。
//
// 支持的类型：
//   - 基础类型：string / int / uint / float / bool（含底层为基础类型的自定义类型）
//   - 指针类型：自动分配内存后设置
//   - 切片类型：逗号分隔，如 default:"1,2,3" → []int，default:"a,b,c" → []string
//   - time.Duration：如 default:"5s"
//   - time.Time：如 default:"2006-01-02" 或 default:"2006-01-02 15:04:05"
//   - 自定义类型：实现 encoding.TextUnmarshaler 接口
//
// 例：
//
//	type ListReq struct {
//		Page int    `query:"page" default:"1"`
//		Size int    `query:"size" default:"20"`
//		Sort string `query:"sort" default:"desc"`
//		IDs  []int  `query:"ids" default:"1,2,3"`
//	}
func ApplyDefaults(obj any) {
	if obj == nil {
		return
	}
	applyDefaults(reflect.ValueOf(obj))
}

func applyDefaults(rv reflect.Value) {
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

		def := field.Tag.Get("default")
		if def == "" || def == "-" {
			// 无默认值标签时，递归处理嵌套结构体
			if fv.Kind() == reflect.Struct || fv.Kind() == reflect.Ptr {
				applyDefaults(fv)
			}
			continue
		}

		if !fv.CanSet() || !fv.IsZero() {
			continue
		}
		setFieldDefault(fv, def)
	}
}

func setFieldDefault(fv reflect.Value, val string) {
	// 1. 指针类型：分配内存后递归设置
	if fv.Kind() == reflect.Ptr {
		if fv.IsNil() {
			fv.Set(reflect.New(fv.Type().Elem()))
		}
		setFieldDefault(fv.Elem(), val)
		return
	}

	// 2. time.Duration
	if fv.Type() == reflect.TypeOf(time.Duration(0)) {
		if d, err := time.ParseDuration(val); err == nil {
			fv.SetInt(int64(d))
		}
		return
	}

	// 3. time.Time（尝试多种常见格式）
	if fv.Type() == reflect.TypeOf(time.Time{}) {
		if t, ok := parseTime(val); ok {
			fv.Set(reflect.ValueOf(t))
		}
		return
	}

	// 4. 切片类型：逗号分隔
	if fv.Kind() == reflect.Slice {
		setSliceDefault(fv, val)
		return
	}

	// 5. 自定义类型：优先走 TextUnmarshaler 接口
	if u, ok := fv.Interface().(encoding.TextUnmarshaler); ok {
		if err := u.UnmarshalText([]byte(val)); err == nil {
			return
		}
	}
	if fv.CanAddr() {
		if u, ok := fv.Addr().Interface().(encoding.TextUnmarshaler); ok {
			if err := u.UnmarshalText([]byte(val)); err == nil {
				return
			}
		}
	}

	// 6. 基础类型（含底层为基础类型的自定义类型）
	SetFieldFromString(fv, val)
}

func setSliceDefault(fv reflect.Value, val string) {
	parts := strings.Split(val, ",")
	et := fv.Type().Elem()
	s := reflect.MakeSlice(fv.Type(), 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		ev := reflect.New(et).Elem()
		setFieldDefault(ev, p)
		s = reflect.Append(s, ev)
	}
	fv.Set(s)
}

func parseTime(val string) (time.Time, bool) {
	layouts := []string{
		time.RFC3339,
		time.DateTime,
		time.DateOnly,
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, val, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
