// Package base 提供 HTTP 层的共享实现，供 fiber / gin 驱动包引用，
// 同时避免与顶层 framework/http 包产生循环依赖。
package base

import (
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/utils"
)

// Response 是 GoFast 标准 JSON 响应结构。
// 它实现了 contracts.Response，可通过 ctx.Response() 获取并直接发送。
type Response struct {
	ctx     contracts.Context `json:"-"`
	Code    int               `json:"code"`
	Message string            `json:"message"`
	Data    any               `json:"data,omitempty"`
}

// NewResponse 为当前请求上下文创建一个响应构建器。
func NewResponse(ctx contracts.Context) *Response {
	return &Response{
		ctx:     ctx,
		Code:    0,
		Message: "ok",
	}
}

// Build 构建并发送完整响应。
func (r *Response) Build(status int, code int, message string, data any) error {
	r.Code = code
	r.Message = message
	r.Data = data
	return r.ctx.JSON(status, r)
}

// Json 快速返回任意 JSON 响应（业务码固定为 0）。
func (r *Response) Json(status int, data any, message ...string) error {
	msg := "ok"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	return r.Build(status, 0, msg, data)
}

// String 快速返回纯文本响应（HTTP 200）。
func (r *Response) String(s string) error {
	return r.ctx.String(http.StatusOK, s)
}

// File 直接输出存储中的文件内容，默认使用 storage 默认磁盘。
func (r *Response) File(file string, disk ...string) error {
	storage := r.ctx.Storage()
	if storage == nil {
		return r.Fail(http.StatusInternalServerError, "storage not available")
	}

	var driver contracts.StorageDriver = storage
	if len(disk) > 0 && disk[0] != "" {
		driver = storage.Disk(disk[0])
	}

	if driver.Missing(file) {
		return r.NotFound("文件不存在")
	}

	if mime, err := driver.MimeType(file); err == nil && mime != "" {
		r.ctx.SetHeader("Content-Type", mime)
	}

	return r.ctx.SendFile(driver.Path(file))
}

// Download 以附件下载方式输出文件，可自定义下载文件名。
func (r *Response) Download(file string, name string, disk ...string) error {
	storage := r.ctx.Storage()
	if storage == nil {
		return r.Fail(http.StatusInternalServerError, "storage not available")
	}

	var driver contracts.StorageDriver = storage
	if len(disk) > 0 && disk[0] != "" {
		driver = storage.Disk(disk[0])
	}

	if driver.Missing(file) {
		return r.NotFound("文件不存在")
	}

	filename := name
	if filename == "" {
		filename = filepath.Base(file)
	}
	r.ctx.SetHeader("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	if mime, err := driver.MimeType(file); err == nil && mime != "" {
		r.ctx.SetHeader("Content-Type", mime)
	}
	return r.ctx.SendFile(driver.Path(file))
}

// Success 快速返回成功响应（HTTP 200, code=0）。
func (r *Response) Success(data any, message ...string) error {
	msg := "ok"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	return r.Build(http.StatusOK, 0, msg, data)
}

// Fail 快速返回失败响应（业务码默认 0）。
func (r *Response) Fail(status int, message string, code ...int) error {
	bizCode := 0
	if len(code) > 0 {
		bizCode = code[0]
	}
	return r.Build(status, bizCode, message, nil)
}

// Created 快速返回创建成功响应（HTTP 201, code=0）。
func (r *Response) Created(data any, message ...string) error {
	msg := "ok"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	return r.Build(http.StatusCreated, 0, msg, data)
}

// Unauthorized 快速返回未授权响应（HTTP 401）。
func (r *Response) Unauthorized(message ...string) error {
	msg := "未授权"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	return r.Fail(http.StatusUnauthorized, msg)
}

// Forbidden 快速返回无权限响应（HTTP 403）。
func (r *Response) Forbidden(message ...string) error {
	msg := "无权限访问"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	return r.Fail(http.StatusForbidden, msg)
}

// NotFound 快速返回资源不存在响应（HTTP 404）。
func (r *Response) NotFound(message ...string) error {
	msg := "资源不存在"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	return r.Fail(http.StatusNotFound, msg)
}

// Validation 快速返回参数验证失败响应（HTTP 422）。
func (r *Response) Validation(err error, message ...string) error {
	msg := "参数验证失败"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	if err == nil {
		return r.Fail(http.StatusUnprocessableEntity, msg)
	}
	return r.Build(http.StatusUnprocessableEntity, 1, msg, map[string]any{
		"error": err.Error(),
	})
}

// Paginate 快速返回分页数据响应（HTTP 200, code=0）。
func (r *Response) Paginate(list any, total int64, page int, size int, message ...string) error {
	msg := "ok"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	page, size = utils.PageUtil.Normalize(page, size)
	return r.Build(http.StatusOK, 0, msg, map[string]any{
		"list":  list,
		"total": total,
		"page":  page,
		"size":  size,
	})
}

// Write 写入原始字节到响应体。
func (r *Response) Write(data []byte) error {
	return r.ctx.String(http.StatusOK, string(data))
}

// View 渲染 HTML 模板并发送 HTTP 200 响应。
// name 为相对于模板目录的路径，例如 "home/index.html"。
func (r *Response) View(name string, data any) error {
	return r.ctx.HTML(http.StatusOK, name, data)
}

// Errorf 便于把格式化错误快速传递给上层调用。
func Errorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

