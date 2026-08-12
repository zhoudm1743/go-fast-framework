package gin

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// route 实现 contracts.Route，封装 Gin。
type route struct {
	engine     *gin.Engine
	router     gin.IRouter
	cfg        contracts.Config
	validator  contracts.Validation
	storage    contracts.Storage
	log        contracts.Log
	server     *http.Server
	viewEngine contracts.ViewEngine
}

// NewRoute 创建基于 Gin 的 HTTP 路由服务实例。
func NewRoute(cfg contracts.Config, validator contracts.Validation, storage contracts.Storage, log contracts.Log) (contracts.Route, error) {
	mode := cfg.GetString("server.mode", "debug")
	if mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}
	// 禁用 gin 默认的控制台输出，由框架 log 统一处理
	gin.DefaultWriter = newLogWriter(log, false)
	gin.DefaultErrorWriter = newLogWriter(log, true)

	engine := gin.New()

	// Recovery 中间件：使用框架 log 记录 panic
	engine.Use(recoveryMiddleware(log))
	// Logger 中间件：使用框架 log 记录每次请求
	engine.Use(loggerMiddleware(log))
	// 请求 ID 中间件
	engine.Use(requestIDMiddleware())

	// CORS 中间件
	allowOrigins := "*"
	if originsRaw := cfg.Get("server.cors_allow_origins"); originsRaw != nil {
		if origins, ok := originsRaw.([]any); ok && len(origins) > 0 {
			allowOrigins = ""
			for i, o := range origins {
				if i > 0 {
					allowOrigins += ","
				}
				allowOrigins += fmt.Sprintf("%v", o)
			}
		}
	}
	engine.Use(corsMiddleware(allowOrigins))

	// 健康检查
	engine.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r := &route{
		engine:    engine,
		router:    engine,
		cfg:       cfg,
		validator: validator,
		storage:   storage,
		log:       log,
	}
	return r, nil
}

// SetViewEngine 将 HTML 模板引擎注入路由，后续每个请求上下文将持有该引擎引用。
func (r *route) SetViewEngine(ve contracts.ViewEngine) {
	r.viewEngine = ve
}

func (r *route) Run(addr ...string) error {
	address := fmt.Sprintf("%s:%d",
		r.cfg.GetString("server.host", "0.0.0.0"),
		r.cfg.GetInt("server.port", 3000))
	if len(addr) > 0 && addr[0] != "" {
		address = addr[0]
	}

	r.log.Infof("[GoFast/gin] listening on %s", address)

	srv := &http.Server{
		Addr:         address,
		Handler:      r.engine,
		ReadTimeout:  time.Duration(r.cfg.GetInt("server.read_timeout_sec", 30)) * time.Second,
		WriteTimeout: time.Duration(r.cfg.GetInt("server.write_timeout_sec", 30)) * time.Second,
		IdleTimeout:  time.Duration(r.cfg.GetInt("server.idle_timeout_sec", 120)) * time.Second,
	}
	r.server = srv
	return srv.ListenAndServe()
}

func (r *route) Shutdown() error {
	if r.server == nil {
		return nil
	}
	r.log.Info("[GoFast/gin] graceful stop...")
	timeout := time.Duration(r.cfg.GetInt("server.shutdown_timeout_sec", 10)) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return r.server.Shutdown(ctx)
}

func (r *route) Get(path string, h contracts.HandlerFunc) contracts.Route {
	r.router.Handle(http.MethodGet, path, r.wrap(h))
	return r
}
func (r *route) Post(path string, h contracts.HandlerFunc) contracts.Route {
	r.router.Handle(http.MethodPost, path, r.wrap(h))
	return r
}
func (r *route) Put(path string, h contracts.HandlerFunc) contracts.Route {
	r.router.Handle(http.MethodPut, path, r.wrap(h))
	return r
}
func (r *route) Delete(path string, h contracts.HandlerFunc) contracts.Route {
	r.router.Handle(http.MethodDelete, path, r.wrap(h))
	return r
}
func (r *route) Patch(path string, h contracts.HandlerFunc) contracts.Route {
	r.router.Handle(http.MethodPatch, path, r.wrap(h))
	return r
}
func (r *route) Head(path string, h contracts.HandlerFunc) contracts.Route {
	r.router.Handle(http.MethodHead, path, r.wrap(h))
	return r
}
func (r *route) Options(path string, h contracts.HandlerFunc) contracts.Route {
	r.router.Handle(http.MethodOptions, path, r.wrap(h))
	return r
}

func (r *route) Group(prefix string, args ...any) contracts.Route {
	ginGroup := r.router.Group(prefix)

	newRoute := &route{
		engine:     r.engine,
		router:     ginGroup,
		cfg:        r.cfg,
		validator:  r.validator,
		storage:    r.storage,
		log:        r.log,
		server:     r.server,
		viewEngine: r.viewEngine,
	}

	var callback func(contracts.Route)

	for _, arg := range args {
		switch v := arg.(type) {
		case contracts.HandlerFunc:
			ginGroup.Use(r.wrap(v))
		case func(contracts.Context) error:
			ginGroup.Use(r.wrap(v))
		case func(contracts.Route):
			callback = v
		}
	}

	if callback != nil {
		callback(newRoute)
	}

	return newRoute
}

func (r *route) Use(middleware ...contracts.HandlerFunc) contracts.Route {
	for _, m := range middleware {
		r.router.Use(r.wrap(m))
	}
	return r
}

func (r *route) Register(controllers ...contracts.Controller) contracts.Route {
	for _, c := range controllers {
		var target contracts.Route = r

		if pc, ok := c.(contracts.Prefixer); ok {
			if p := pc.Prefix(); p != "" {
				target = target.Group(p)
			}
		}

		if mc, ok := c.(contracts.Middlewarer); ok {
			if m := mc.Middleware(); len(m) > 0 {
				target.Use(m...)
			}
		}

		c.Boot(target)
	}
	return r
}

// Static 从本地目录 dir 提供静态文件服务。
func (r *route) Static(urlPrefix, dir string) contracts.Route {
	r.router.Static(urlPrefix, dir)
	return r
}

// StaticFS 从任意 http.FileSystem 提供静态文件服务（支持 http.FS(embed.FS)）。
func (r *route) StaticFS(urlPrefix string, fs http.FileSystem) contracts.Route {
	r.router.StaticFS(urlPrefix, fs)
	return r
}

// SPA 挂载单页应用（全站兜底，必须最后注册）。
func (r *route) SPA(fsys fs.FS, root string) contracts.Route {
	sub := fsys
	if root != "" && root != "." {
		s, err := fs.Sub(fsys, root)
		if err != nil {
			r.log.Warnf("[GoFast/gin] SPA disabled: fs.Sub(%q) failed: %v", root, err)
			return r
		}
		sub = s
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		r.log.Warnf("[GoFast/gin] SPA disabled: index.html not found")
		return r
	}
	fileServer := http.FileServer(http.FS(sub))
	r.engine.NoRoute(func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		p := strings.TrimPrefix(path.Clean("/"+c.Request.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if f, err := sub.Open(p); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}
		c.Request.URL.Path = "/"
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
	r.log.Info("[GoFast/gin] SPA mounted on /")
	return r
}

// StaticSPA 在指定 URL 前缀挂载单页应用。
func (r *route) StaticSPA(prefix string, fsys fs.FS, root string) contracts.Route {
	sub := fsys
	if root != "" && root != "." {
		s, err := fs.Sub(fsys, root)
		if err != nil {
			r.log.Warnf("[GoFast/gin] StaticSPA disabled: fs.Sub(%q) failed: %v", root, err)
			return r
		}
		sub = s
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		r.log.Warnf("[GoFast/gin] StaticSPA disabled: index.html not found")
		return r
	}
	prefix = "/" + strings.Trim(prefix, "/")
	fileServer := http.FileServer(http.FS(sub))
	handler := func(c *gin.Context) {
		p := strings.TrimPrefix(path.Clean("/"+c.Request.URL.Path), "/")
		if f, err := sub.Open(p); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}
		c.Request.URL.Path = "/"
		fileServer.ServeHTTP(c.Writer, c.Request)
	}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		r.engine.Handle(method, prefix, handler)
		r.engine.Handle(method, prefix+"/*filepath", handler)
	}
	r.log.Infof("[GoFast/gin] StaticSPA mounted on %s", prefix)
	return r
}

// wrap 将 contracts.HandlerFunc 转为 Gin handler。
func (r *route) wrap(h contracts.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := h(NewContext(c, r.validator, r.storage, r.viewEngine)); err != nil {
			_ = c.Error(err)
		}
	}
}

// Routes 返回所有已注册路由的基本信息（Method + Path）。
func (r *route) Routes() []contracts.RouteInfo {
	ginRoutes := r.engine.Routes()
	seen := make(map[string]struct{}, len(ginRoutes))
	var result []contracts.RouteInfo
	for _, gr := range ginRoutes {
		key := gr.Method + ":" + gr.Path
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, contracts.RouteInfo{
			Method: gr.Method,
			Path:   gr.Path,
		})
	}
	if result == nil {
		result = []contracts.RouteInfo{}
	}
	return result
}

// Route 返回指定路径的路由信息，不存在时返回零值 RouteInfo。
func (r *route) Route(path string) contracts.RouteInfo {
	for _, gr := range r.engine.Routes() {
		if gr.Path == path {
			return contracts.RouteInfo{
				Method: gr.Method,
				Path:   gr.Path,
			}
		}
	}
	return contracts.RouteInfo{}
}

// ── 内置中间件 ──────────────────────────────────────────────────────

// loggerMiddleware 记录每个请求的方法、路径、状态码和耗时。
func loggerMiddleware(log contracts.Log) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		if c.Request.URL.RawQuery != "" {
			path += "?" + c.Request.URL.RawQuery
		}

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method

		entry := log.WithFields(map[string]any{
			"status":  status,
			"latency": latency.String(),
			"ip":      clientIP,
			"method":  method,
			"path":    path,
		})

		switch {
		case status >= http.StatusInternalServerError:
			entry.Error("[GoFast/gin]")
		case status >= http.StatusBadRequest:
			entry.Warn("[GoFast/gin]")
		default:
			entry.Info("[GoFast/gin]")
		}
	}
}

// recoveryMiddleware 捕获 panic 并通过框架 log 记录堆栈，返回 500。
func recoveryMiddleware(log contracts.Log) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				stack := debug.Stack()
				log.WithFields(map[string]any{
					"error": fmt.Sprintf("%v", err),
					"stack": string(stack),
				}).Error("[GoFast/gin] panic recovered")
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		c.Next()
	}
}

func corsMiddleware(allowOrigins string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", allowOrigins)
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,PATCH,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin,Content-Type,Accept,Authorization")
		c.Header("Access-Control-Max-Age", "86400")
		if allowOrigins != "*" {
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = fmt.Sprintf("%d", time.Now().UnixNano())
		}
		c.Header("X-Request-ID", id)
		c.Set("requestid", id)
		c.Next()
	}
}

// ── logWriter：将 gin 内部输出重定向到框架 log ──────────────────────

type logWriter struct {
	log   contracts.Log
	isErr bool
}

func newLogWriter(log contracts.Log, isErr bool) *logWriter {
	return &logWriter{log: log, isErr: isErr}
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	msg := fmt.Sprintf("[GoFast/gin] %s", strings.TrimRight(string(p), "\n\r"))
	if w.isErr {
		w.log.Error(msg)
	} else {
		w.log.Debug(msg)
	}
	return len(p), nil
}
