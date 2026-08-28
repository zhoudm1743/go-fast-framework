package utils

import (
	"strconv"
	"strings"
	"time"
)

// ConvertUtil 类型转换工具集。
var ConvertUtil = convertUtil{}

type convertUtil struct{}

func (r convertUtil) ToInt(s string, defaultValue ...int) int {
	def := 0
	if len(defaultValue) > 0 {
		def = defaultValue[0]
	}
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return n
}

func (r convertUtil) ToInt64(s string, defaultValue ...int64) int64 {
	def := int64(0)
	if len(defaultValue) > 0 {
		def = defaultValue[0]
	}
	if s == "" {
		return def
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return def
	}
	return n
}

func (r convertUtil) ToUint64(s string, defaultValue ...uint64) uint64 {
	def := uint64(0)
	if len(defaultValue) > 0 {
		def = defaultValue[0]
	}
	if s == "" {
		return def
	}
	n, err := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return def
	}
	return n
}

func (r convertUtil) ToFloat64(s string, defaultValue ...float64) float64 {
	def := float64(0)
	if len(defaultValue) > 0 {
		def = defaultValue[0]
	}
	if s == "" {
		return def
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return def
	}
	return f
}

func (r convertUtil) ToBool(s string, defaultValue ...bool) bool {
	def := false
	if len(defaultValue) > 0 {
		def = defaultValue[0]
	}
	if s == "" {
		return def
	}
	b, err := strconv.ParseBool(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return b
}

func (r convertUtil) ToString(v any) string {
	return r.ToStringFrom(v)
}

func (r convertUtil) ToStringFrom(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	case bool:
		return strconv.FormatBool(val)
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	default:
		return ""
	}
}

func (r convertUtil) ToDuration(s string, defaultValue ...time.Duration) time.Duration {
	def := time.Duration(0)
	if len(defaultValue) > 0 {
		def = defaultValue[0]
	}
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return d
}

func (r convertUtil) ToBytes(s string) []byte {
	return []byte(s)
}

func (r convertUtil) ToIntSlice(s string, sep ...string) []int {
	d := ","
	if len(sep) > 0 && sep[0] != "" {
		d = sep[0]
	}
	parts := strings.Split(s, d)
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if n, err := strconv.Atoi(p); err == nil {
			out = append(out, n)
		}
	}
	return out
}

func (r convertUtil) ToStringSlice(s string, sep ...string) []string {
	d := ","
	if len(sep) > 0 && sep[0] != "" {
		d = sep[0]
	}
	parts := strings.Split(s, d)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (r convertUtil) ParseInt(s string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(s))
}

func (r convertUtil) ParseInt64(s string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(s), 10, 64)
}

func (r convertUtil) ParseFloat64(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

func (r convertUtil) ParseBool(s string) (bool, error) {
	return strconv.ParseBool(strings.TrimSpace(s))
}

func (r convertUtil) BitBool(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (r convertUtil) BoolBit(n int) bool {
	return n != 0
}
