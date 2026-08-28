package utils

import (
	"strings"
)

// Dict 创建 map 链式操作（泛型）。
func Dict[K comparable, V any](m map[K]V) *DictChain[K, V] {
	cp := make(map[K]V, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return &DictChain[K, V]{data: cp}
}

// DictAny 创建 map[string]any 链式操作。
func DictAny(m map[string]any) *DictAnyChain {
	cp := make(map[string]any, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return &DictAnyChain{data: cp}
}

// DictChain map 链式操作。
type DictChain[K comparable, V any] struct {
	data map[K]V
}

func (d *DictChain[K, V]) Only(keys ...K) *DictChain[K, V] {
	set := make(map[K]struct{}, len(keys))
	for _, k := range keys {
		set[k] = struct{}{}
	}
	out := make(map[K]V, len(keys))
	for k, v := range d.data {
		if _, ok := set[k]; ok {
			out[k] = v
		}
	}
	d.data = out
	return d
}

func (d *DictChain[K, V]) Except(keys ...K) *DictChain[K, V] {
	set := make(map[K]struct{}, len(keys))
	for _, k := range keys {
		set[k] = struct{}{}
	}
	out := make(map[K]V)
	for k, v := range d.data {
		if _, ok := set[k]; !ok {
			out[k] = v
		}
	}
	d.data = out
	return d
}

func (d *DictChain[K, V]) Set(key K, value V) *DictChain[K, V] {
	d.data[key] = value
	return d
}

func (d *DictChain[K, V]) Merge(other map[K]V) *DictChain[K, V] {
	for k, v := range other {
		d.data[k] = v
	}
	return d
}

func (d *DictChain[K, V]) Clone() *DictChain[K, V] {
	return Dict(d.data)
}

func (d *DictChain[K, V]) Has(key K) bool {
	_, ok := d.data[key]
	return ok
}

func (d *DictChain[K, V]) Keys() []K {
	keys := make([]K, 0, len(d.data))
	for k := range d.data {
		keys = append(keys, k)
	}
	return keys
}

func (d *DictChain[K, V]) Values() []V {
	vals := make([]V, 0, len(d.data))
	for _, v := range d.data {
		vals = append(vals, v)
	}
	return vals
}

func (d *DictChain[K, V]) Map() map[K]V {
	cp := make(map[K]V, len(d.data))
	for k, v := range d.data {
		cp[k] = v
	}
	return cp
}

// DictAnyChain map[string]any 链式操作，支持点路径。
type DictAnyChain struct {
	data map[string]any
}

func (d *DictAnyChain) Only(keys ...string) *DictAnyChain {
	set := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		set[k] = struct{}{}
	}
	out := make(map[string]any, len(keys))
	for k, v := range d.data {
		if _, ok := set[k]; ok {
			out[k] = v
		}
	}
	d.data = out
	return d
}

func (d *DictAnyChain) Except(keys ...string) *DictAnyChain {
	set := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		set[k] = struct{}{}
	}
	out := make(map[string]any)
	for k, v := range d.data {
		if _, ok := set[k]; !ok {
			out[k] = v
		}
	}
	d.data = out
	return d
}

func (d *DictAnyChain) Set(key string, value any) *DictAnyChain {
	d.data[key] = value
	return d
}

func (d *DictAnyChain) Forget(keys ...string) *DictAnyChain {
	for _, k := range keys {
		delete(d.data, k)
	}
	return d
}

func (d *DictAnyChain) Merge(other map[string]any) *DictAnyChain {
	for k, v := range other {
		d.data[k] = v
	}
	return d
}

func (d *DictAnyChain) DeepMerge(other map[string]any) *DictAnyChain {
	deepMergeMap(d.data, other)
	return d
}

func (d *DictAnyChain) Get(path string, def ...any) any {
	v := mapGet(d.data, path)
	if v == nil && len(def) > 0 {
		return def[0]
	}
	return v
}

func (d *DictAnyChain) Has(key string) bool {
	_, ok := d.data[key]
	return ok
}

func (d *DictAnyChain) Keys() []string {
	keys := make([]string, 0, len(d.data))
	for k := range d.data {
		keys = append(keys, k)
	}
	return keys
}

func (d *DictAnyChain) Values() []any {
	vals := make([]any, 0, len(d.data))
	for _, v := range d.data {
		vals = append(vals, v)
	}
	return vals
}

func (d *DictAnyChain) Flatten(prefix string) *DictAnyChain {
	d.data = flattenMap(d.data, prefix, "")
	return d
}

func (d *DictAnyChain) Map() map[string]any {
	cp := make(map[string]any, len(d.data))
	for k, v := range d.data {
		cp[k] = v
	}
	return cp
}

func mapGet(m map[string]any, path string) any {
	if path == "" {
		return nil
	}
	parts := strings.Split(path, ".")
	var cur any = m
	for _, p := range parts {
		sub, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur, ok = sub[p]
		if !ok {
			return nil
		}
	}
	return cur
}

func mapSet(m map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	cur := m
	for i, p := range parts {
		if i == len(parts)-1 {
			cur[p] = value
			return
		}
		next, ok := cur[p].(map[string]any)
		if !ok {
			next = make(map[string]any)
			cur[p] = next
		}
		cur = next
	}
}

func deepMergeMap(dst, src map[string]any) {
	for k, v := range src {
		if dv, ok := dst[k].(map[string]any); ok {
			if sv, ok := v.(map[string]any); ok {
				deepMergeMap(dv, sv)
				continue
			}
		}
		dst[k] = v
	}
}

func flattenMap(m map[string]any, prefix, parent string) map[string]any {
	out := make(map[string]any)
	for k, v := range m {
		key := k
		if parent != "" {
			key = parent + prefix + k
		}
		if sub, ok := v.(map[string]any); ok {
			for fk, fv := range flattenMap(sub, prefix, key) {
				out[fk] = fv
			}
		} else {
			out[key] = v
		}
	}
	return out
}

func unflattenMap(m map[string]any, sep string) map[string]any {
	out := make(map[string]any)
	for k, v := range m {
		parts := strings.Split(k, sep)
		cur := out
		for i, p := range parts {
			if i == len(parts)-1 {
				cur[p] = v
				break
			}
			next, ok := cur[p].(map[string]any)
			if !ok {
				next = make(map[string]any)
				cur[p] = next
			}
			cur = next
		}
	}
	return out
}
