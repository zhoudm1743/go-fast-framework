// Package http 提供框架无关的 HTTP 抽象层。
// Response 实现已迁移至 framework/http/base，此处保留类型别名以向后兼容。
package http

import "github.com/zhoudm1743/go-fast-framework/http/base"

// Response 是 GoFast 标准 JSON 响应结构（类型别名，实现在 base 包）。
type Response = base.Response

// NewResponse 为当前请求上下文创建一个响应构建器。
var NewResponse = base.NewResponse

// Errorf 格式化错误辅助函数。
var Errorf = base.Errorf
