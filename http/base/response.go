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
//
// 语义变更（响应双写根治）：发送成功返回哨兵 contracts.ErrResponseSent 而非 nil。
// 此前“写出成功返回 nil”导致 `if err := resp.NotFound(...); err != nil` 形式的
// 业务封装误判为“未响应”，调用方继续执行又写一次响应，最终响应体出现两段拼接
// JSON。返回哨兵后，调用方把哨兵以 error 上抛，路由层（fiber/gin 的 wrap）识别
// 到哨兵即结束请求、不再二次渲染；业务方可用 contracts.IsResponseSent 区分
// “已响应”与真实错误。发送失败返回底层真实错误（不吞错、不包哨兵）。
func (r *Response) Build(status int, code int, message string, data any) error {
	r.Code = code
	r.Message = message
	r.Data = data
	if err := r.ctx.JSON(status, r); err != nil {
		return err // 底层写失败是真实错误，原样上抛
	}
	return contracts.ErrResponseSent // 成功写出 → 哨兵，路由层据此结束请求
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
// 与 Build 一致：成功返回 ErrResponseSent 哨兵，失败返回底层错误。
func (r *Response) String(s string) error {
	if err := r.ctx.String(http.StatusOK, s); err != nil {
		return err
	}
	return contracts.ErrResponseSent
}

// File 直接输出存储中的文件内容，默认使用 storage 默认磁盘。
// 内部的 r.Fail / r.NotFound 写出响应后返回哨兵，会自然向上传播，无需重复包装；
// SendFile 成功同样返回哨兵，失败返回底层错误。
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

	if err := r.ctx.SendFile(driver.Path(file)); err != nil {
		return err
	}
	return contracts.ErrResponseSent
}

// Download 以附件下载方式输出文件，可自定义下载文件名。
// 内部的 r.Fail / r.NotFound 写出响应后返回哨兵，会自然向上传播，无需重复包装；
// SendFile 成功同样返回哨兵，失败返回底层错误。
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
	if err := r.ctx.SendFile(driver.Path(file)); err != nil {
		return err
	}
	return contracts.ErrResponseSent
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
// 与 Build 一致：成功返回 ErrResponseSent 哨兵，失败返回底层错误。
func (r *Response) Write(data []byte) error {
	if err := r.ctx.String(http.StatusOK, string(data)); err != nil {
		return err
	}
	return contracts.ErrResponseSent
}

// View 渲染 HTML 模板并发送 HTTP 200 响应。
// name 为相对于模板目录的路径，例如 "home/index.html"。
// 与 Build 一致：成功返回 ErrResponseSent 哨兵，失败返回底层错误。
func (r *Response) View(name string, data any) error {
	if err := r.ctx.HTML(http.StatusOK, name, data); err != nil {
		return err
	}
	return contracts.ErrResponseSent
}

// Errorf 便于把格式化错误快速传递给上层调用。
func Errorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}
