package utils

import (
	"reflect"
)

// HelperUtil 小函数工具集。
var HelperUtil = helperUtil{}

type helperUtil struct{}

// If 泛型三元表达式，推荐优先使用 HelperIf。
func HelperIf[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}

func (r helperUtil) If(cond bool, a, b any) any {
	if cond {
		return a
	}
	return b
}

// Or 返回第一个非零值。
func HelperOr[T comparable](values ...T) T {
	var zero T
	for _, v := range values {
		if v != zero {
			return v
		}
	}
	return zero
}

func (r helperUtil) Or(values ...any) any {
	for _, v := range values {
		if !r.Empty(v) {
			return v
		}
	}
	return nil
}

// Coalesce 同 Or。
func HelperCoalesce[T comparable](values ...T) T {
	return HelperOr(values...)
}

func (r helperUtil) Coalesce(values ...any) any {
	return r.Or(values...)
}

// Empty 判断值是否为空。
func (r helperUtil) Empty(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.String, reflect.Array:
		return rv.Len() == 0
	case reflect.Slice, reflect.Map:
		return rv.IsNil() || rv.Len() == 0
	case reflect.Ptr, reflect.Interface:
		return rv.IsNil()
	default:
		return rv.IsZero()
	}
}

// Default 零值时返回 fallback。
func HelperDefault[T comparable](v, fallback T) T {
	var zero T
	if v == zero {
		return fallback
	}
	return v
}

func (r helperUtil) Default(v, fallback any) any {
	if r.Empty(v) {
		return fallback
	}
	return v
}

// Must err 非空则 panic。
func (r helperUtil) Must(_ any, err error) {
	if err != nil {
		panic(err)
	}
}

// Ignore 显式丢弃 error。
func (r helperUtil) Ignore(err error) {
	_ = err
}
