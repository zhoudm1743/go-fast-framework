package base

import "strconv"

// QueryInt 解析字符串为 int，空值或解析失败时返回默认值（默认 0）。
func QueryInt(val string, defaultValue ...int) int {
	def := 0
	if len(defaultValue) > 0 {
		def = defaultValue[0]
	}
	if val == "" {
		return def
	}
	if n, err := strconv.Atoi(val); err == nil {
		return n
	}
	return def
}

// QueryInt64 解析字符串为 int64，空值或解析失败时返回默认值（默认 0）。
func QueryInt64(val string, defaultValue ...int64) int64 {
	def := int64(0)
	if len(defaultValue) > 0 {
		def = defaultValue[0]
	}
	if val == "" {
		return def
	}
	if n, err := strconv.ParseInt(val, 10, 64); err == nil {
		return n
	}
	return def
}

// QueryFloat64 解析字符串为 float64，空值或解析失败时返回默认值（默认 0）。
func QueryFloat64(val string, defaultValue ...float64) float64 {
	def := float64(0)
	if len(defaultValue) > 0 {
		def = defaultValue[0]
	}
	if val == "" {
		return def
	}
	if f, err := strconv.ParseFloat(val, 64); err == nil {
		return f
	}
	return def
}

// QueryBool 解析字符串为 bool，空值或解析失败时返回默认值（默认 false）。
func QueryBool(val string, defaultValue ...bool) bool {
	def := false
	if len(defaultValue) > 0 {
		def = defaultValue[0]
	}
	if val == "" {
		return def
	}
	if b, err := strconv.ParseBool(val); err == nil {
		return b
	}
	return def
}
