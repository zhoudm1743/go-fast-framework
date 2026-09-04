package gin

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zhoudm1743/go-fast-framework/http/cors"
)

func newCORSTestEngine(opts cors.Options) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(corsMiddleware(opts))
	engine.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})
	return engine
}

func doCORSTest(engine *gin.Engine, method, origin string, extraHeaders map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/ping", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func TestCorsWildcard(t *testing.T) {
	engine := newCORSTestEngine(cors.Options{
		AllowOrigins: []string{"*"},
		AllowMethods: "GET,POST",
		AllowHeaders: "Origin,Content-Type",
		MaxAge:       600,
	})

	w := doCORSTest(engine, http.MethodGet, "https://a.com", nil)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("通配时期望 ACAO=*，实际 %q", got)
	}
	if w.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Error("通配 + 未启用凭据时不应输出 ACAC")
	}
	if got := w.Header().Get("Access-Control-Allow-Methods"); got != "GET,POST" {
		t.Errorf("期望 ACAM=GET,POST，实际 %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Headers"); got != "Origin,Content-Type" {
		t.Errorf("期望 ACAH=Origin,Content-Type，实际 %q", got)
	}
	if got := w.Header().Get("Access-Control-Max-Age"); got != "600" {
		t.Errorf("期望 Max-Age=600，实际 %q", got)
	}
	if w.Code != http.StatusOK || w.Body.String() != "pong" {
		t.Errorf("非预检请求应透传 handler，实际 %d %q", w.Code, w.Body.String())
	}
}

func TestCorsSingleOriginEchoesMatchedOrigin(t *testing.T) {
	engine := newCORSTestEngine(cors.Options{
		AllowOrigins:     []string{"https://a.com"},
		AllowMethods:     "GET",
		AllowHeaders:     "Content-Type",
		AllowCredentials: true,
		MaxAge:           600,
	})

	w := doCORSTest(engine, http.MethodGet, "https://a.com", nil)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://a.com" {
		t.Errorf("应回显命中的 Origin，实际 %q", got)
	}
	if w.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Error("启用凭据时应输出 ACAC=true")
	}
	if got := w.Header().Values("Vary"); len(got) == 0 {
		t.Error("非通配时应输出 Vary: Origin")
	}
}

func TestCorsMultiOriginEchoesMatchedOriginOnly(t *testing.T) {
	// 回归：v0.7.12 把 "https://a.com,https://b.com" 整串塞进单个 ACAO 头，违反 CORS 规范
	engine := newCORSTestEngine(cors.Options{
		AllowOrigins: []string{"https://a.com", "https://b.com"},
		AllowMethods: "GET",
	})

	for _, origin := range []string{"https://a.com", "https://b.com"} {
		w := doCORSTest(engine, http.MethodGet, origin, nil)
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != origin {
			t.Errorf("多 origin 场景应逐请求回显 %q，实际 %q", origin, got)
		}
	}
}

func TestCorsOriginNotInWhitelistGetsNoCorsHeaders(t *testing.T) {
	engine := newCORSTestEngine(cors.Options{
		AllowOrigins: []string{"https://a.com"},
		AllowMethods: "GET",
	})

	w := doCORSTest(engine, http.MethodGet, "https://evil.com", nil)
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("未命中白名单的 Origin 不应获得 ACAO 头")
	}
	if w.Code != http.StatusOK {
		t.Errorf("服务端仍应正常处理请求（拦截交给浏览器），实际 %d", w.Code)
	}
}

func TestCorsPreflight(t *testing.T) {
	engine := newCORSTestEngine(cors.Options{
		AllowOrigins: []string{"https://a.com"},
		AllowMethods: "GET,POST",
		AllowHeaders: "X-Token",
		MaxAge:       600,
	})

	w := doCORSTest(engine, http.MethodOptions, "https://a.com", map[string]string{
		"Access-Control-Request-Method": "POST",
	})
	if w.Code != http.StatusNoContent {
		t.Errorf("预检请求期望 204，实际 %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "https://a.com" {
		t.Errorf("预检响应应回显命中 Origin，实际 %q", w.Header().Get("Access-Control-Allow-Origin"))
	}
	if w.Header().Get("Access-Control-Allow-Methods") != "GET,POST" {
		t.Errorf("预检应输出 ACAM，实际 %q", w.Header().Get("Access-Control-Allow-Methods"))
	}
	if w.Header().Get("Access-Control-Allow-Headers") != "X-Token" {
		t.Errorf("预检应输出 ACAH，实际 %q", w.Header().Get("Access-Control-Allow-Headers"))
	}
}

func TestCorsWildcardWithCredentialsEchoesOrigin(t *testing.T) {
	// 规范禁止 ACAO:* 搭配凭据，启用凭据后应回显请求 Origin
	engine := newCORSTestEngine(cors.Options{
		AllowOrigins:     []string{"*"},
		AllowMethods:     "GET",
		AllowCredentials: true,
	})

	w := doCORSTest(engine, http.MethodGet, "https://a.com", nil)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://a.com" {
		t.Errorf("通配 + 凭据时应回显请求 Origin，实际 %q", got)
	}
	if w.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Error("应输出 ACAC=true")
	}
}

func TestCorsExposeHeadersAndMaxAgeOptional(t *testing.T) {
	engine := newCORSTestEngine(cors.Options{
		AllowOrigins: []string{"*"},
		AllowMethods: "GET",
	})

	w := doCORSTest(engine, http.MethodGet, "https://a.com", nil)
	if w.Header().Get("Access-Control-Expose-Headers") != "" {
		t.Error("未配置 ExposeHeaders 时不应输出该头")
	}
	if w.Header().Get("Access-Control-Max-Age") != "" {
		t.Error("MaxAge<=0 时不应输出 Max-Age 头")
	}

	engine = newCORSTestEngine(cors.Options{
		AllowOrigins:  []string{"*"},
		AllowMethods:  "GET",
		ExposeHeaders: "X-Total-Count",
	})
	w = doCORSTest(engine, http.MethodGet, "https://a.com", nil)
	if got := w.Header().Get("Access-Control-Expose-Headers"); got != "X-Total-Count" {
		t.Errorf("期望暴露 X-Total-Count，实际 %q", got)
	}
}

func newSecurityHeadersEngine(hstsMaxAge int) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(securityHeadersMiddleware(hstsMaxAge))
	engine.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})
	return engine
}

func TestSecurityHeadersEnabled(t *testing.T) {
	engine := newSecurityHeadersEngine(31536000)

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if got := w.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options 期望 DENY，实际 %q", got)
	}
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options 期望 nosniff，实际 %q", got)
	}
	if got := w.Header().Get("Referrer-Policy"); got != "strict-origin-when-cross-origin" {
		t.Errorf("Referrer-Policy 期望 strict-origin-when-cross-origin，实际 %q", got)
	}
	// HTTP 请求不输出 HSTS
	if got := w.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("HTTP 请求不应输出 HSTS，实际 %q", got)
	}
}

func TestSecurityHeadersHSTSOnTLS(t *testing.T) {
	engine := newSecurityHeadersEngine(31536000)

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.TLS = &tls.ConnectionState{}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if got := w.Header().Get("Strict-Transport-Security"); got != "max-age=31536000" {
		t.Errorf("HTTPS 请求应输出 HSTS，实际 %q", got)
	}
}

func TestSecurityHeadersHSTSDisabled(t *testing.T) {
	engine := newSecurityHeadersEngine(0)

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.TLS = &tls.ConnectionState{}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if got := w.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("hsts_max_age<=0 时不应输出 HSTS，实际 %q", got)
	}
}
