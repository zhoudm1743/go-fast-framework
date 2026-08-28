package utils

import (
	"reflect"
)

// MapUtil map 短函数工具集。
var MapUtil = mapUtil{}

type mapUtil struct{}

func (r mapUtil) Keys(m map[string]any) []string {
	return DictAny(m).Keys()
}

func (r mapUtil) Values(m map[string]any) []any {
	return DictAny(m).Values()
}

func (r mapUtil) Has(m map[string]any, key string) bool {
	_, ok := m[key]
	return ok
}

func (r mapUtil) Get(m map[string]any, path string, def ...any) any {
	return DictAny(m).Get(path, def...)
}

func (r mapUtil) Only(m map[string]any, keys ...string) map[string]any {
	return DictAny(m).Only(keys...).Map()
}

func (r mapUtil) Except(m map[string]any, keys ...string) map[string]any {
	return DictAny(m).Except(keys...).Map()
}

func (r mapUtil) Merge(a, b map[string]any) map[string]any {
	out := make(map[string]any, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func (r mapUtil) DeepMerge(a, b map[string]any) map[string]any {
	out := make(map[string]any, len(a))
	for k, v := range a {
		out[k] = v
	}
	deepMergeMap(out, b)
	return out
}

func (r mapUtil) Clone(m map[string]any) map[string]any {
	return DictAny(m).Map()
}

func (r mapUtil) Equal(a, b map[string]any) bool {
	return reflect.DeepEqual(a, b)
}

func (r mapUtil) Flatten(m map[string]any, sep ...string) map[string]any {
	prefix := "."
	if len(sep) > 0 {
		prefix = sep[0]
	}
	return flattenMap(m, prefix, "")
}

func (r mapUtil) Unflatten(m map[string]any, sep ...string) map[string]any {
	s := "."
	if len(sep) > 0 {
		s = sep[0]
	}
	return unflattenMap(m, s)
}

func (r mapUtil) Invert(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[v] = k
	}
	return out
}

func (r mapUtil) Filter(m map[string]any, fn func(k string, v any) bool) map[string]any {
	out := make(map[string]any)
	for k, v := range m {
		if fn(k, v) {
			out[k] = v
		}
	}
	return out
}

func (r mapUtil) MapValues(m map[string]any, fn func(k string, v any) any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = fn(k, v)
	}
	return out
}

func (r mapUtil) Set(m map[string]any, path string, value any) map[string]any {
	cp := r.Clone(m)
	mapSet(cp, path, value)
	return cp
}

func (r mapUtil) Forget(m map[string]any, keys ...string) map[string]any {
	return DictAny(m).Forget(keys...).Map()
}
