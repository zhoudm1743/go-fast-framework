package base

import (
	"reflect"

	"github.com/zhoudm1743/go-fast-framework/utils"
)

// SetFieldFromString 将字符串写入反射字段（支持基础类型及自定义底层类型）。
// 数值类型解析失败时不修改字段；字符串类型不做 Trim。
func SetFieldFromString(fv reflect.Value, val string) {
	if !fv.CanSet() {
		return
	}
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(val)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if n, err := utils.ConvertUtil.ParseInt64(val); err == nil {
			fv.SetInt(n)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if n, err := utils.ConvertUtil.ParseInt64(val); err == nil && n >= 0 {
			fv.SetUint(uint64(n))
		}
	case reflect.Float32, reflect.Float64:
		if f, err := utils.ConvertUtil.ParseFloat64(val); err == nil {
			fv.SetFloat(f)
		}
	case reflect.Bool:
		if b, err := utils.ConvertUtil.ParseBool(val); err == nil {
			fv.SetBool(b)
		}
	}
}
