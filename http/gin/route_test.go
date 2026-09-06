package gin

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zhoudm1743/go-fast-framework/contracts"
)

func TestWrapAbortOnErrorWithResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := &route{}
	engine := gin.New()

	controllerCalled := false
	middleware := func(ctx contracts.Context) error {
		return ctx.Response().Unauthorized("未授权")
	}
	controller := func(ctx contracts.Context) error {
		controllerCalled = true
		return ctx.Response().Success("ok")
	}

	engine.GET("/test", r.wrap(middleware), r.wrap(controller))

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test", nil))

	if controllerCalled {
		t.Error("中间件返回 error 后 controller 不应执行")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("期望 401，实际 %d", w.Code)
	}
	body := w.Body.String()
	if strings.Count(body, "{") != 1 {
		t.Errorf("响应体应只有一个 JSON，实际 %q", body)
	}
	if strings.Contains(body, "ok") {
		t.Errorf("响应体不应包含 controller 输出，实际 %q", body)
	}
}

func TestWrapAbortOnErrorWithoutResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := &route{}
	engine := gin.New()

	controllerCalled := false
	middleware := func(ctx contracts.Context) error {
		return errors.New("boom")
	}
	controller := func(ctx contracts.Context) error {
		controllerCalled = true
		return ctx.Response().Success("ok")
	}

	engine.GET("/test", r.wrap(middleware), r.wrap(controller))

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test", nil))

	if controllerCalled {
		t.Error("中间件返回 error 后 controller 不应执行")
	}
	if w.Code != http.StatusInternalServerError {
		t.Errorf("未写响应的 error 应补 500，实际 %d", w.Code)
	}
}

func TestWrapAbortStopsMultipleHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := &route{}
	engine := gin.New()

	var secondCalled, thirdCalled bool
	first := func(ctx contracts.Context) error {
		return ctx.Response().Unauthorized("未授权")
	}
	second := func(ctx contracts.Context) error {
		secondCalled = true
		return ctx.Response().Unauthorized("未授权2")
	}
	third := func(ctx contracts.Context) error {
		thirdCalled = true
		return ctx.Response().Success("ok")
	}

	engine.GET("/test", r.wrap(first), r.wrap(second), r.wrap(third))

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test", nil))

	if secondCalled || thirdCalled {
		t.Errorf("第一个中间件中断后，后续 handler 不应执行: second=%v third=%v", secondCalled, thirdCalled)
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("期望 401，实际 %d", w.Code)
	}
	if strings.Count(w.Body.String(), "{") != 1 {
		t.Errorf("响应体应只有一个 JSON，实际 %q", w.Body.String())
	}
}

func TestWrapNextContinues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := &route{}
	engine := gin.New()

	controllerCalled := false
	middleware := func(ctx contracts.Context) error {
		return ctx.Next()
	}
	controller := func(ctx contracts.Context) error {
		controllerCalled = true
		return ctx.Response().Success("ok")
	}

	engine.GET("/test", r.wrap(middleware), r.wrap(controller))

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test", nil))

	if !controllerCalled {
		t.Error("中间件 Next 后 controller 应执行")
	}
	if w.Code != http.StatusOK {
		t.Errorf("期望 200，实际 %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "ok") {
		t.Errorf("期望响应包含 ok，实际 %q", w.Body.String())
	}
}

// 复现响应双写缺陷场景：业务 helper 内部已写出 404 并把结果以 error 上抛。
// 旧版 NotFound 成功后返回 nil → err==nil → 调用方继续执行又写一次，响应体
// 出现两段拼接 JSON；新版返回哨兵 ErrResponseSent，路由层识别后直接结束请求。
func TestWrapRecognizesResponseSentinelNoDoubleWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := &route{}
	engine := gin.New()

	notFound := func(ctx contracts.Context) error {
		return ctx.Response().NotFound("订单不存在")
	}
	handler := func(ctx contracts.Context) error {
		if err := notFound(ctx); err != nil {
			return err // 业务把“已响应”当 error 原样上抛（缺陷触发路径）
		}
		return ctx.Response().Success("ok") // 双写缺陷下会被执行的第二次写出
	}

	// 记录 wrap 是否把哨兵当错误记入 c.Errors（哨兵不是错误，不应记录）
	var cErrorCount int
	recorder := func(c *gin.Context) {
		c.Next()
		cErrorCount = len(c.Errors)
	}

	engine.GET("/sentinel", recorder, r.wrap(handler))

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/sentinel", nil))

	if w.Code != http.StatusNotFound {
		t.Errorf("期望 404（第一次写出的状态码），实际 %d", w.Code)
	}
	body := w.Body.String()
	if strings.Count(body, "{") != 1 {
		t.Errorf("响应体应只有一段 JSON（无二次写出拼接），实际 %q", body)
	}
	if !strings.Contains(body, "订单不存在") {
		t.Errorf("响应体应保留第一次写出的内容，实际 %q", body)
	}
	if strings.Contains(body, "ok") || strings.Contains(body, "服务器内部错误") {
		t.Errorf("不应出现第二次写出或 500 文本，实际 %q", body)
	}
	if cErrorCount != 0 {
		t.Errorf("哨兵不是错误，不应记入 c.Errors，实际 %d 条", cErrorCount)
	}
}

// 哨兵被业务层 %w 包装后仍需被路由层识别（errors.Is 解包）。
func TestWrapRecognizesWrappedResponseSentinel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := &route{}
	engine := gin.New()

	handler := func(ctx contracts.Context) error {
		if err := ctx.Response().Unauthorized("token 过期"); err != nil {
			return fmt.Errorf("鉴权失败: %w", err)
		}
		return nil
	}

	engine.GET("/wrapped-sentinel", r.wrap(handler))

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/wrapped-sentinel", nil))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("期望 401，实际 %d", w.Code)
	}
	body := w.Body.String()
	if strings.Count(body, "{") != 1 || !strings.Contains(body, "token 过期") {
		t.Errorf("响应体应只有一段 401 JSON，实际 %q", body)
	}
	if strings.Contains(body, "服务器内部错误") {
		t.Errorf("被包装的哨兵不应触发 500，实际 %q", body)
	}
}
