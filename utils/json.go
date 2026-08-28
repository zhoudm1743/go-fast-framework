package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// JsonUtil JSON 工具集。
var JsonUtil = jsonUtil{}

type jsonUtil struct{}

// JSON 链式 JSON 对象。
type JSON struct {
	raw  []byte
	data any
	err  error
}

func (r jsonUtil) Of(v any) *JSON {
	b, err := json.Marshal(v)
	return &JSON{raw: b, data: v, err: err}
}

func (r jsonUtil) Parse(src any) *JSON {
	j := &JSON{}
	switch v := src.(type) {
	case string:
		j.raw = []byte(v)
	case []byte:
		j.raw = v
	default:
		j.err = fmt.Errorf("json: unsupported source type")
		return j
	}
	if err := json.Unmarshal(j.raw, &j.data); err != nil {
		j.err = err
	}
	return j
}

func (r jsonUtil) Encode(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (r jsonUtil) Decode(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func (r jsonUtil) Valid(data []byte) bool {
	return json.Valid(data)
}

func (j *JSON) Get(path string, def ...any) *JSON {
	if j.err != nil {
		return j
	}
	m, ok := j.data.(map[string]any)
	if !ok {
		m2, ok := j.toMap()
		if !ok {
			j.err = fmt.Errorf("json: not an object")
			return j
		}
		m = m2
	}
	v := mapGet(m, path)
	if v == nil && len(def) > 0 {
		j.data = def[0]
	} else {
		j.data = v
	}
	return j
}

func (j *JSON) Set(path string, value any) *JSON {
	if j.err != nil {
		return j
	}
	m, ok := j.toMap()
	if !ok {
		m = make(map[string]any)
	}
	mapSet(m, path, value)
	j.data = m
	b, err := json.Marshal(m)
	j.raw = b
	j.err = err
	return j
}

func (j *JSON) Merge(other map[string]any) *JSON {
	if j.err != nil {
		return j
	}
	m, ok := j.toMap()
	if !ok {
		m = make(map[string]any)
	}
	deepMergeMap(m, other)
	j.data = m
	b, err := json.Marshal(m)
	j.raw = b
	j.err = err
	return j
}

func (j *JSON) Pretty() *JSON {
	if j.err != nil {
		return j
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, j.raw, "", "  "); err != nil {
		j.err = err
		return j
	}
	j.raw = buf.Bytes()
	return j
}

func (j *JSON) ToMap() (map[string]any, error) {
	if j.err != nil {
		return nil, j.err
	}
	m, ok := j.toMap()
	if !ok {
		return nil, fmt.Errorf("json: not an object")
	}
	return m, nil
}

func (j *JSON) ToSlice() ([]any, error) {
	if j.err != nil {
		return nil, j.err
	}
	s, ok := j.data.([]any)
	if !ok {
		return nil, fmt.Errorf("json: not an array")
	}
	return s, nil
}

func (j *JSON) ToString() (string, error) {
	if j.err != nil {
		return "", j.err
	}
	if s, ok := j.data.(string); ok {
		return s, nil
	}
	b, err := json.Marshal(j.data)
	return string(b), err
}

func (j *JSON) ToInt() (int, error) {
	if j.err != nil {
		return 0, j.err
	}
	switch v := j.data.(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	case json.Number:
		i, err := v.Int64()
		return int(i), err
	default:
		return ConvertUtil.ParseInt(fmt.Sprint(j.data))
	}
}

func (j *JSON) Bytes() []byte {
	return j.raw
}

func (j *JSON) String() string {
	if len(j.raw) > 0 {
		return string(j.raw)
	}
	s, _ := j.ToString()
	return s
}

func (j *JSON) Err() error {
	return j.err
}

func (j *JSON) toMap() (map[string]any, bool) {
	if m, ok := j.data.(map[string]any); ok {
		return m, true
	}
	return nil, false
}
