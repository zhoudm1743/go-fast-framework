package utils

import (
	"encoding/json"
	"fmt"
	"reflect"
)

func (r structUtil) DeepCopy(dst, src any) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

func (r structUtil) Merge(dst, src any) error {
	dm := r.StructToMap(dst)
	sm := r.StructToMap(src)
	for k, v := range sm {
		dm[k] = v
	}
	return r.mapToStruct(dm, dst)
}

func (r structUtil) Diff(a, b any) map[string]any {
	ma := r.StructToMap(a)
	mb := r.StructToMap(b)
	out := make(map[string]any)
	for k, va := range ma {
		if vb, ok := mb[k]; !ok || !reflect.DeepEqual(va, vb) {
			out[k] = va
		}
	}
	for k, vb := range mb {
		if _, ok := ma[k]; !ok {
			out[k] = vb
		}
	}
	return out
}

func (r structUtil) Fields(obj any) []string {
	rv := reflect.ValueOf(obj)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}
	rt := rv.Type()
	fields := make([]string, 0, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.PkgPath != "" {
			continue
		}
		fields = append(fields, r.key(f))
	}
	return fields
}

func (r structUtil) mapToStruct(m map[string]any, dst any) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, dst); err != nil {
		return fmt.Errorf("mapToStruct: %w", err)
	}
	return nil
}
