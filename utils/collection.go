package utils

import (
	"slices"
)

// Collect 创建切片链式集合。
func Collect[T any](items []T) *Collection[T] {
	cp := make([]T, len(items))
	copy(cp, items)
	return &Collection[T]{items: cp}
}

// Collection 切片链式操作。
type Collection[T any] struct {
	items []T
}

func (c *Collection[T]) Filter(fn func(T) bool) *Collection[T] {
	out := make([]T, 0, len(c.items))
	for _, v := range c.items {
		if fn(v) {
			out = append(out, v)
		}
	}
	c.items = out
	return c
}

func (c *Collection[T]) Map(fn func(T) T) *Collection[T] {
	out := make([]T, len(c.items))
	for i, v := range c.items {
		out[i] = fn(v)
	}
	c.items = out
	return c
}

func (c *Collection[T]) Flat(fn func(T) []T) *Collection[T] {
	out := make([]T, 0)
	for _, v := range c.items {
		out = append(out, fn(v)...)
	}
	c.items = out
	return c
}

func (c *Collection[T]) Reverse() *Collection[T] {
	slices.Reverse(c.items)
	return c
}

func (c *Collection[T]) Unique() *Collection[T] {
	seen := make(map[any]struct{})
	out := make([]T, 0, len(c.items))
	for _, v := range c.items {
		key := any(v)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, v)
	}
	c.items = out
	return c
}

func (c *Collection[T]) UniqueBy(keyFn func(T) any) *Collection[T] {
	seen := make(map[any]struct{})
	out := make([]T, 0, len(c.items))
	for _, v := range c.items {
		key := keyFn(v)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, v)
	}
	c.items = out
	return c
}

func (c *Collection[T]) SortBy(less func(a, b T) bool) *Collection[T] {
	slices.SortFunc(c.items, func(a, b T) int {
		if less(a, b) {
			return -1
		}
		if less(b, a) {
			return 1
		}
		return 0
	})
	return c
}

func (c *Collection[T]) Shuffle() *Collection[T] {
	// Fisher-Yates using RandomUtil when available; simple swap for now
	for i := len(c.items) - 1; i > 0; i-- {
		j := RandomUtil.Intn(i + 1)
		c.items[i], c.items[j] = c.items[j], c.items[i]
	}
	return c
}

func (c *Collection[T]) Concat(other []T) *Collection[T] {
	c.items = append(c.items, other...)
	return c
}

func (c *Collection[T]) Diff(other []T) *Collection[T] {
	set := make(map[any]struct{}, len(other))
	for _, v := range other {
		set[any(v)] = struct{}{}
	}
	out := make([]T, 0)
	for _, v := range c.items {
		if _, ok := set[any(v)]; !ok {
			out = append(out, v)
		}
	}
	c.items = out
	return c
}

func (c *Collection[T]) Contains(v T) bool {
	for _, item := range c.items {
		if any(item) == any(v) {
			return true
		}
	}
	return false
}

func (c *Collection[T]) Index(v T) int {
	for i, item := range c.items {
		if any(item) == any(v) {
			return i
		}
	}
	return -1
}

func (c *Collection[T]) First() (T, bool) {
	if len(c.items) == 0 {
		var zero T
		return zero, false
	}
	return c.items[0], true
}

func (c *Collection[T]) Last() (T, bool) {
	if len(c.items) == 0 {
		var zero T
		return zero, false
	}
	return c.items[len(c.items)-1], true
}

func (c *Collection[T]) Nth(n int) (T, bool) {
	if n < 0 || n >= len(c.items) {
		var zero T
		return zero, false
	}
	return c.items[n], true
}

func (c *Collection[T]) Len() int {
	return len(c.items)
}

func (c *Collection[T]) Empty() bool {
	return len(c.items) == 0
}

func (c *Collection[T]) Slice() []T {
	cp := make([]T, len(c.items))
	copy(cp, c.items)
	return cp
}

func (c *Collection[T]) Reduce(initial T, fn func(acc, v T) T) T {
	acc := initial
	for _, v := range c.items {
		acc = fn(acc, v)
	}
	return acc
}

func (c *Collection[T]) Sample(n int) []T {
	if n >= len(c.items) {
		return c.Slice()
	}
	cp := c.Slice()
	c2 := Collect(cp).Shuffle()
	return c2.items[:n]
}

func (c *Collection[T]) Pick() (T, bool) {
	if len(c.items) == 0 {
		var zero T
		return zero, false
	}
	idx := RandomUtil.Intn(len(c.items))
	return c.items[idx], true
}

// Pluck 从结构体切片抽取字段（需传入 getter）。
func Pluck[T any, R any](items []T, getter func(T) R) []R {
	out := make([]R, len(items))
	for i, v := range items {
		out[i] = getter(v)
	}
	return out
}

// KeyBy 按 key 建立 map。
func KeyBy[T any, K comparable](items []T, keyFn func(T) K) map[K]T {
	out := make(map[K]T, len(items))
	for _, v := range items {
		out[keyFn(v)] = v
	}
	return out
}

// GroupBy 分组。
func GroupBy[T any, K comparable](items []T, keyFn func(T) K) map[K][]T {
	out := make(map[K][]T)
	for _, v := range items {
		k := keyFn(v)
		out[k] = append(out[k], v)
	}
	return out
}

// CountBy 计数。
func CountBy[T any, K comparable](items []T, keyFn func(T) K) map[K]int {
	out := make(map[K]int)
	for _, v := range items {
		out[keyFn(v)]++
	}
	return out
}

// Intersect 交集。
func Intersect[T comparable](a, b []T) []T {
	set := make(map[T]struct{}, len(b))
	for _, v := range b {
		set[v] = struct{}{}
	}
	out := make([]T, 0)
	for _, v := range a {
		if _, ok := set[v]; ok {
			out = append(out, v)
		}
	}
	return out
}

// Union 并集。
func Union[T comparable](a, b []T) []T {
	return Collect(a).Concat(b).Unique().Slice()
}
