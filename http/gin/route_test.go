package gin

import (
	"errors"
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
