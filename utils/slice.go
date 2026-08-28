package utils

import "reflect"

// SliceUtil 切片短函数工具集（基于反射，支持任意切片类型）。
var SliceUtil = sliceUtil{}

type sliceUtil struct{}

func (r sliceUtil) Contains(items any, v any) bool {
	rv := reflect.ValueOf(items)
	if rv.Kind() != reflect.Slice {
		return false
	}
	for i := 0; i < rv.Len(); i++ {
		if reflect.DeepEqual(rv.Index(i).Interface(), v) {
			return true
		}
	}
	return false
}

func (r sliceUtil) In(v any, items ...any) bool {
	if len(items) == 1 {
		if rv := reflect.ValueOf(items[0]); rv.Kind() == reflect.Slice {
			return r.Contains(items[0], v)
		}
	}
	for _, item := range items {
		if reflect.DeepEqual(item, v) {
			return true
		}
	}
	return false
}

func (r sliceUtil) Index(items any, v any) int {
	rv := reflect.ValueOf(items)
	if rv.Kind() != reflect.Slice {
		return -1
	}
	for i := 0; i < rv.Len(); i++ {
		if reflect.DeepEqual(rv.Index(i).Interface(), v) {
			return i
		}
	}
	return -1
}

func (r sliceUtil) Unique(items any) any {
	rv := reflect.ValueOf(items)
	if rv.Kind() != reflect.Slice {
		return items
	}
	seen := make(map[any]struct{})
	out := reflect.MakeSlice(rv.Type(), 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		v := rv.Index(i).Interface()
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = reflect.Append(out, rv.Index(i))
	}
	return out.Interface()
}

func (r sliceUtil) Reverse(items any) any {
	rv := reflect.ValueOf(items)
	if rv.Kind() != reflect.Slice {
		return items
	}
	out := reflect.MakeSlice(rv.Type(), rv.Len(), rv.Len())
	for i := 0; i < rv.Len(); i++ {
		out.Index(i).Set(rv.Index(rv.Len() - 1 - i))
	}
	return out.Interface()
}

func (r sliceUtil) First(items any) (any, bool) {
	rv := reflect.ValueOf(items)
	if rv.Kind() != reflect.Slice || rv.Len() == 0 {
		return nil, false
	}
	return rv.Index(0).Interface(), true
}

func (r sliceUtil) Last(items any) (any, bool) {
	rv := reflect.ValueOf(items)
	if rv.Kind() != reflect.Slice || rv.Len() == 0 {
		return nil, false
	}
	return rv.Index(rv.Len() - 1).Interface(), true
}

func (r sliceUtil) Chunk(items any, size int) any {
	rv := reflect.ValueOf(items)
	if rv.Kind() != reflect.Slice || size <= 0 {
		return items
	}
	elemType := rv.Type().Elem()
	sliceOfSlice := reflect.SliceOf(reflect.SliceOf(elemType))
	var chunks reflect.Value
	chunks = reflect.MakeSlice(sliceOfSlice, 0, 0)
	for i := 0; i < rv.Len(); i += size {
		end := i + size
		if end > rv.Len() {
			end = rv.Len()
		}
		chunks = reflect.Append(chunks, rv.Slice(i, end))
	}
	return chunks.Interface()
}

func (r sliceUtil) Diff(a, b any) any {
	return diffReflect(a, b)
}

func (r sliceUtil) Intersect(a, b any) any {
	return intersectReflect(a, b)
}

func diffReflect(a, b any) any {
	ra, rb := reflect.ValueOf(a), reflect.ValueOf(b)
	if ra.Kind() != reflect.Slice || rb.Kind() != reflect.Slice {
		return a
	}
	set := make(map[any]struct{})
	for i := 0; i < rb.Len(); i++ {
		set[rb.Index(i).Interface()] = struct{}{}
	}
	out := reflect.MakeSlice(ra.Type(), 0, ra.Len())
	for i := 0; i < ra.Len(); i++ {
		v := ra.Index(i).Interface()
		if _, ok := set[v]; !ok {
			out = reflect.Append(out, ra.Index(i))
		}
	}
	return out.Interface()
}

func intersectReflect(a, b any) any {
	ra, rb := reflect.ValueOf(a), reflect.ValueOf(b)
	if ra.Kind() != reflect.Slice || rb.Kind() != reflect.Slice {
		return a
	}
	set := make(map[any]struct{})
	for i := 0; i < rb.Len(); i++ {
		set[rb.Index(i).Interface()] = struct{}{}
	}
	out := reflect.MakeSlice(ra.Type(), 0, 0)
	for i := 0; i < ra.Len(); i++ {
		v := ra.Index(i).Interface()
		if _, ok := set[v]; ok {
			out = reflect.Append(out, ra.Index(i))
		}
	}
	return out.Interface()
}
