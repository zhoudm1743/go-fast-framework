package utils

import (
	"fmt"
	"reflect"
)

// PtrUtil 指针工具集，配合 StructUtil.PtrToMap 部分更新场景。
var PtrUtil = ptrUtil{}

type ptrUtil struct{}

// Of 返回 v 的指针（基于反射，适用于任意类型）。
func (r ptrUtil) Of(v any) any {
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
	ptr := reflect.New(rv.Type())
	ptr.Elem().Set(rv)
	return ptr.Interface()
}

// PtrOf 泛型取指针，类型安全，推荐优先使用。
func PtrOf[T any](v T) *T {
	return &v
}

// Value 解引用指针，nil 时返回零值。
func (r ptrUtil) Value(p any) any {
	if p == nil {
		return nil
	}
	rv := reflect.ValueOf(p)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return nil
	}
	return rv.Elem().Interface()
}

// PtrValue 泛型解引用。
func PtrValue[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

// ValueOr nil 时返回 def。
func (r ptrUtil) ValueOr(p any, def any) any {
	if p == nil {
		return def
	}
	rv := reflect.ValueOf(p)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return def
	}
	return rv.Elem().Interface()
}

// PtrValueOr 泛型解引用或默认值。
func PtrValueOr[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}

// IsNil 判断指针是否为 nil。
func (r ptrUtil) IsNil(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Interface, reflect.Chan, reflect.Func:
		return rv.IsNil()
	default:
		return false
	}
}

// Must 若 err 非空则 panic。
func Must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// MustErr 若 err 非空则 panic。
func MustErr(err error) {
	if err != nil {
		panic(err)
	}
}

// MustValue 若 err 非空则 panic。
func MustValue(err error) {
	if err != nil {
		panic(fmt.Errorf("%w", err))
	}
}
