package middleware

import (
	"strconv"
	"testing"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/foundation"
	"github.com/zhoudm1743/go-fast-framework/mock"
	"github.com/zhoudm1743/go-fast-framework/ratelimit"
)

// rlCtx 限流中间件测试用 Context。
type rlCtx struct {
	contracts.Context
	ip       string
	fullPath string
	values   map[string]any
	headers  map[string]string
	resp     *rlResponse
	next     bool
}

// rlResponse 记录 429 响应状态的测试用 Response。
type rlResponse struct {
	contracts.Response
	status  int
	message string
}

func (r *rlResponse) Fail(status int, message string, code ...int) error {
	r.status = status
	r.message = message
	return nil
}

func (c *rlCtx) IP() string       { return c.ip }
func (c *rlCtx) FullPath() string { return c.fullPath }
func (c *rlCtx) Value(key string) any {
	return c.values[key]
}
func (c *rlCtx) Response() contracts.Response { return c.resp }
func (c *rlCtx) Next() error                  { c.next = true; return nil }
func (c *rlCtx) SetHeader(k, v string) contracts.Context {
	c.headers[k] = v
	return c
}

func newRateLimitMiddlewareApp(t *testing.T) foundation.Application {
	t.Helper()
	app := foundation.NewApplication(".")
	app.Instance("config", &mwTestConfig{m: map[string]any{}})
	app.Instance("cache", mock.NewMockCache())
	app.Instance("ratelimit", ratelimit.New(app.MustMake("cache").(contracts.Cache), ""))
	return app
}

func TestRateLimitMiddleware_AllowsWithinLimit(t *testing.T) {
	app := newRateLimitMiddlewareApp(t)
	setFacadeApp(t, app)

	mw := RateLimit(3, time.Minute)
	for i := 0; i < 3; i++ {
		ctx := &rlCtx{ip: "1.2.3.4", fullPath: "/login", values: map[string]any{}, headers: map[string]string{}, resp: &rlResponse{}}
		if err := mw(ctx); err != nil {
			t.Fatalf("call %d: err = %v", i+1, err)
		}
		if !ctx.next {
			t.Fatalf("call %d: next not called", i+1)
		}
	}
}

func TestRateLimitMiddleware_BlocksOverLimit(t *testing.T) {
	app := newRateLimitMiddlewareApp(t)
	setFacadeApp(t, app)

	mw := RateLimit(2, time.Minute)
	for i := 0; i < 2; i++ {
		ctx := &rlCtx{ip: "1.2.3.4", fullPath: "/login", values: map[string]any{}, headers: map[string]string{}, resp: &rlResponse{}}
		if err := mw(ctx); err != nil {
			t.Fatal(err)
		}
	}

	ctx := &rlCtx{ip: "1.2.3.4", fullPath: "/login", values: map[string]any{}, headers: map[string]string{}, resp: &rlResponse{}}
	if err := mw(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.next {
		t.Fatal("next must not be called when over limit")
	}
	if ctx.resp.status != 429 {
		t.Fatalf("status = %d, want 429", ctx.resp.status)
	}
	n, err := strconv.Atoi(ctx.headers["Retry-After"])
	if err != nil || n < 1 {
		t.Fatalf("Retry-After = %q, want positive integer", ctx.headers["Retry-After"])
	}
	if n > 60 {
		t.Fatalf("Retry-After = %d, want <= 60 for 1m window", n)
	}
}

func TestRateLimitMiddleware_DefaultKeyIsIPPlusFullPath(t *testing.T) {
	app := newRateLimitMiddlewareApp(t)
	setFacadeApp(t, app)

	mw := RateLimit(1, time.Minute)
	// 同一 IP 不同路径 → 各自独立放行
	for _, p := range []string{"/a", "/b"} {
		ctx := &rlCtx{ip: "1.2.3.4", fullPath: p, values: map[string]any{}, headers: map[string]string{}, resp: &rlResponse{}}
		if err := mw(ctx); err != nil || !ctx.next {
			t.Fatalf("path %s: err=%v next=%v", p, err, ctx.next)
		}
	}
	// 不同 IP 同一路径 → 各自独立放行
	ctx := &rlCtx{ip: "5.6.7.8", fullPath: "/a", values: map[string]any{}, headers: map[string]string{}, resp: &rlResponse{}}
	if err := mw(ctx); err != nil || !ctx.next {
		t.Fatalf("different IP: err=%v next=%v", err, ctx.next)
	}
}

func TestRateLimitMiddleware_CustomKeyFn(t *testing.T) {
	app := newRateLimitMiddlewareApp(t)
	setFacadeApp(t, app)

	mw := RateLimit(1, time.Minute, func(ctx contracts.Context) string {
		return ctx.Value("uid").(string)
	})

	first := &rlCtx{ip: "1.2.3.4", fullPath: "/x", values: map[string]any{"uid": "u1"}, headers: map[string]string{}, resp: &rlResponse{}}
	if err := mw(first); err != nil || !first.next {
		t.Fatalf("first: err=%v next=%v", err, first.next)
	}

	// 相同自定义 key 的不同来源请求必须被拦截
	second := &rlCtx{ip: "9.9.9.9", fullPath: "/y", values: map[string]any{"uid": "u1"}, headers: map[string]string{}, resp: &rlResponse{}}
	if err := mw(second); err != nil {
		t.Fatal(err)
	}
	if second.next {
		t.Fatal("same custom key from different IP/path must be blocked")
	}
}
