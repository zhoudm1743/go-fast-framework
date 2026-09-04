// Package cors 提供 gin / fiber 双驱动共享的 CORS 配置解析与 Origin 匹配逻辑，
// 避免 route.go 中各写一份解析代码。
package cors

import (
	"fmt"
	"strings"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// Options CORS 配置选项，由 Load 从 server.cors_* 配置键解析而来。
type Options struct {
	// AllowOrigins 允许的源列表；为空或包含 "*" 表示放行所有源。
	AllowOrigins []string
	// AllowMethods 放行的 HTTP 方法，逗号分隔（Access-Control-Allow-Methods 值）。
	AllowMethods string
	// AllowHeaders 放行的请求头，逗号分隔（Access-Control-Allow-Headers 值）。
	AllowHeaders string
	// ExposeHeaders 允许浏览器脚本读取的响应头，逗号分隔；空表示不输出 Access-Control-Expose-Headers。
	ExposeHeaders string
	// AllowCredentials 是否允许携带凭据（Access-Control-Allow-Credentials: true）。
	// 注意：放行所有源（*）时凭据仍会生效，此时响应逐请求回显 Origin 而非输出 "*"（CORS 规范禁止 * 搭配凭据）。
	AllowCredentials bool
	// MaxAge 预检结果缓存秒数；<=0 表示不输出 Access-Control-Max-Age。
	MaxAge int
}

// Wildcard 报告是否放行所有源。
func (o Options) Wildcard() bool {
	if len(o.AllowOrigins) == 0 {
		return true
	}
	for _, origin := range o.AllowOrigins {
		if origin == "*" {
			return true
		}
	}
	return false
}

// MatchOrigin 匹配请求 Origin：命中返回应回显的 Origin 值（通配时为 "*"），未命中返回 false。
func (o Options) MatchOrigin(origin string) (string, bool) {
	if origin == "" {
		return "", false
	}
	if o.Wildcard() {
		return "*", true
	}
	for _, allowed := range o.AllowOrigins {
		if strings.EqualFold(strings.TrimSpace(allowed), origin) {
			return origin, true
		}
	}
	return "", false
}

// 配置键默认值，与 v0.7.12 的硬编码行为保持一致。
const (
	DefaultAllowOrigins = "*"
	DefaultAllowMethods = "GET,POST,PUT,DELETE,PATCH,OPTIONS"
	DefaultAllowHeaders = "Origin,Content-Type,Accept,Authorization"
	DefaultMaxAge       = 86400
	// defaultHSTSMaxAge 安全头中间件 HSTS 的默认 max-age（一年）。
	defaultHSTSMaxAge = 31536000
)

// Load 从配置解析 CORS 选项，配置键（config/config.yaml）：
//
//	server:
//	  cors_allow_origins: ["*"]                                  # 放行的源，字符串数组或逗号分隔字符串
//	  cors_allow_methods: "GET,POST,PUT,DELETE,PATCH,OPTIONS"    # 放行的 HTTP 方法
//	  cors_allow_headers: "Origin,Content-Type,Accept,Authorization"  # 放行的请求头
//	  cors_expose_headers: "X-Total-Count"                       # 允许浏览器读取的响应头，默认不输出
//	  cors_allow_credentials: true                               # 缺省时：origins 非 * 时自动开启
//	  cors_max_age: 86400                                        # 预检缓存秒数，<=0 不输出该头
func Load(cfg contracts.Config) Options {
	opts := Options{
		AllowOrigins:  toStringSlice(cfg.Get("server.cors_allow_origins"), []string{DefaultAllowOrigins}),
		AllowMethods:  strings.Join(toStringSlice(cfg.Get("server.cors_allow_methods"), []string{DefaultAllowMethods}), ","),
		AllowHeaders:  strings.Join(toStringSlice(cfg.Get("server.cors_allow_headers"), []string{DefaultAllowHeaders}), ","),
		ExposeHeaders: strings.Join(toStringSlice(cfg.Get("server.cors_expose_headers"), nil), ","),
		MaxAge:        cfg.GetInt("server.cors_max_age", DefaultMaxAge),
	}
	// 凭据缺省时保持 v0.7.12 行为：指定了具体源即自动开启；
	// 显式配置（true/false）优先于自动行为。
	if raw := cfg.Get("server.cors_allow_credentials"); raw != nil {
		opts.AllowCredentials = cfg.GetBool("server.cors_allow_credentials")
	} else {
		opts.AllowCredentials = !opts.Wildcard()
	}
	// 允许所有源时凭据会失效（CORS 规范禁止 ACAO:* 搭配凭据），
	// 改为逐请求回显 Origin 的方式支持凭据，两驱动一致。
	return opts
}

// HSTSMaxAge 读取安全头中间件的 HSTS max-age 配置（server.security_hsts_max_age），<=0 表示不输出 HSTS。
func HSTSMaxAge(cfg contracts.Config) int {
	return cfg.GetInt("server.security_hsts_max_age", defaultHSTSMaxAge)
}

// toStringSlice 将配置值规整为字符串切片：
// 兼容 nil、字符串（按逗号分割）、[]string、[]any（YAML 数组），并去除首尾空白与空项。
func toStringSlice(raw any, def []string) []string {
	switch v := raw.(type) {
	case nil:
		return def
	case string:
		return splitAndTrim(v)
	case []string:
		return trimAll(v)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, fmt.Sprintf("%v", item))
		}
		return trimAll(out)
	default:
		return []string{fmt.Sprintf("%v", raw)}
	}
}

// splitAndTrim 按逗号分割字符串并去除首尾空白与空项。
func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// trimAll 去除每项首尾空白并过滤空项。
func trimAll(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
