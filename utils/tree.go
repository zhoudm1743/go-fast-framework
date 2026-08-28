package utils

import (
	"reflect"
)

// TreeUtil 树形结构工具集（链式）。
var TreeUtil = treeUtil{}

type treeUtil struct{}

type Tree struct {
	list         []map[string]any
	idKey        string
	parentKey    string
	childrenKey  string
	tree         []map[string]any
}

func (r treeUtil) Of(list []map[string]any) *Tree {
	cp := make([]map[string]any, len(list))
	for i, m := range list {
		cp[i] = cloneMap(m)
	}
	return &Tree{list: cp, idKey: "id", parentKey: "parent_id", childrenKey: "children"}
}

func (t *Tree) ID(key string) *Tree {
	t.idKey = key
	return t
}

func (t *Tree) Parent(key string) *Tree {
	t.parentKey = key
	return t
}

func (t *Tree) ChildrenKey(key string) *Tree {
	t.childrenKey = key
	return t
}

func (t *Tree) ToTree() *Tree {
	index := make(map[any]map[string]any)
	roots := make([]map[string]any, 0)
	for _, item := range t.list {
		cp := cloneMap(item)
		cp[t.childrenKey] = []map[string]any{}
		index[cp[t.idKey]] = cp
	}
	for _, item := range index {
		pid := item[t.parentKey]
		if pid == nil || pid == "" || pid == 0 {
			roots = append(roots, item)
			continue
		}
		if parent, ok := index[pid]; ok {
			ch := parent[t.childrenKey].([]map[string]any)
			parent[t.childrenKey] = append(ch, item)
		} else {
			roots = append(roots, item)
		}
	}
	t.tree = roots
	return t
}

func (t *Tree) Flatten() []map[string]any {
	if len(t.tree) == 0 {
		t.ToTree()
	}
	var out []map[string]any
	var walk func(nodes []map[string]any)
	walk = func(nodes []map[string]any) {
		for _, n := range nodes {
			cp := cloneMap(n)
			delete(cp, t.childrenKey)
			out = append(out, cp)
			if ch, ok := n[t.childrenKey].([]map[string]any); ok {
				walk(ch)
			}
		}
	}
	walk(t.tree)
	return out
}

func (t *Tree) ChildrenOf(rootID any) []map[string]any {
	if len(t.tree) == 0 {
		t.ToTree()
	}
	var find func(nodes []map[string]any) []map[string]any
	find = func(nodes []map[string]any) []map[string]any {
		for _, n := range nodes {
			if reflect.DeepEqual(n[t.idKey], rootID) {
				if ch, ok := n[t.childrenKey].([]map[string]any); ok {
					return ch
				}
				return nil
			}
			if ch, ok := n[t.childrenKey].([]map[string]any); ok {
				if res := find(ch); res != nil {
					return res
				}
			}
		}
		return nil
	}
	return find(t.tree)
}

func (t *Tree) Leafs() []map[string]any {
	if len(t.tree) == 0 {
		t.ToTree()
	}
	var out []map[string]any
	var walk func(nodes []map[string]any)
	walk = func(nodes []map[string]any) {
		for _, n := range nodes {
			ch, ok := n[t.childrenKey].([]map[string]any)
			if !ok || len(ch) == 0 {
				cp := cloneMap(n)
				delete(cp, t.childrenKey)
				out = append(out, cp)
				continue
			}
			walk(ch)
		}
	}
	walk(t.tree)
	return out
}

func cloneMap(m map[string]any) map[string]any {
	cp := make(map[string]any, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}
