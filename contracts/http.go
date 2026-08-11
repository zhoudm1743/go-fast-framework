package contracts

import (
	"html/template"
	"io"
	"net/http"
	"time"
)

// HandlerFunc 是 GoFast HTTP 处理函数的统一签名。
// 应用层代码只需依赖此类型，无需引入任何底层 HTTP 框架包。
type HandlerFunc func(ctx Context) error

// Response 是统一响应构建器。
// 控制器可通过 ctx.Response() 获取它，快速返回标准 JSON 响应。
type Response interface {
	// Build 构建并发送完整响应。
	Build(status int, code int, message string, data any) error
	// Json 快速返回任意 JSON 响应（自定义 HTTP 状态码，业务码固定为 0）。
	Json(status int, data any, message ...string) error
	// String 快速返回纯文本响应（HTTP 200）。
	String(s string) error
	// File 直接输出存储中的文件内容，默认使用 storage 默认磁盘。
	File(file string, disk ...string) error
	// Download 以附件下载方式输出文件，可自定义下载文件名。
	Download(file string, name string, disk ...string) error
	// Success 快速返回成功响应（HTTP 200, code=0）。
	Success(data any, message ...string) error
	// Fail 快速返回失败响应（默认业务码 code=1）。
	Fail(status int, message string, code ...int) error
	// Created 快速返回创建成功响应（HTTP 201, code=0）。
	Created(data any, message ...string) error
	// Unauthorized 快速返回未授权响应（HTTP 401）。
	Unauthorized(message ...string) error
	// Forbidden 快速返回无权限响应（HTTP 403）。
	Forbidden(message ...string) error
	// NotFound 快速返回资源不存在响应（HTTP 404）。
	NotFound(message ...string) error
	// Validation 快速返回参数验证失败响应（HTTP 422）。
	Validation(err error, message ...string) error
	// Paginate 快速返回分页数据响应（HTTP 200, code=0）。
	Paginate(list any, total int64, page int, size int, message ...string) error
	// View 渲染 HTML 模板并发送 HTTP 200 响应；需先通过 view.ServiceProvider 注册模板引擎。
	// name 为相对于模板目录的路径，例如 "home/index.html"。
	View(name string, data any) error
	// Write 写入响应体。
	Write(data []byte) error
}

// Context 是传递给每个 Handler 和 Middleware 的核心对象。
// 它封装了请求读取、响应发送、上下文存储三大能力。
type Context interface {
	// ── 请求读取 ──────────────────────────────────

	// Method 返回 HTTP 方法（GET / POST / …）。
	Method() string
	// Path 返回请求路径。
	Path() string
	// Param 返回 URL 路径参数，例如路由 "/users/:id" 中的 "id"。
	Param(key string) string
	// Query 返回查询字符串参数，支持默认值。
	Query(key string, defaultValue ...string) string
	// Header 返回请求头的值。
	Header(key string) string
	// IP 返回客户端 IP 地址。
	IP() string
	// BodyRaw 返回原始请求体字节。
	BodyRaw() []byte

	// Bind 将请求体（JSON / Form）反序列化到 obj，并自动执行验证（binding tag）。
	// 验证失败时返回包含字段错误信息的 error。
	Bind(obj any) error

	// ── 文件上传 ───────────────────────────────────
	File(key string) (File, error)
	// Files 返回 multipart 表单中指定 key 的所有上传文件，兼容单文件与多文件。
	// 返回的每个 File 均可直接调用 Store / StoreAs 持久化到任意 Storage 磁盘。
	Files(key string) ([]File, error)

	// ── 文件与存储 ─────────────────────────────────

	// Storage 返回文件存储服务（供 Response.File / Response.Download 使用）。
	Storage() Storage
	// SendFile 直接从文件系统绝对路径发送文件响应（由底层驱动实现）。
	SendFile(path string) error

	// ── 响应发送 ──────────────────────────────────

	// JSON 以 JSON 格式发送响应，code 为 HTTP 状态码。
	JSON(code int, obj any) error
	// String 发送纯文本响应。
	String(code int, s string) error
	// HTML 渲染 HTML 模板并发送响应；需先通过 view.ServiceProvider 注册模板引擎。
	// name 为相对于模板目录的路径，例如 "home/index.html"。
	HTML(code int, name string, data any) error
	// Response 返回统一响应构建器。
	Response() Response
	// Status 设置响应状态码，返回自身以支持链式调用。
	Status(code int) Context
	// SetHeader 设置响应头，返回自身以支持链式调用。
	SetHeader(key, value string) Context
	// Write 写入原始字节到响应体。
	Write(data []byte) error

	// ── 上下文存储（用于 Middleware 传值）────────

	// Value 读取通过 WithValue 存储的键值。
	Value(key string) any
	// WithValue 在当前请求上下文中存储键值对，返回自身。
	WithValue(key string, value any) Context

	// ── Middleware 控制 ───────────────────────────

	// Next 调用调用链中的下一个处理函数（供 Middleware 使用）。
	Next() error
	Abort() error
	AbortWithCode(code int) error
	AbortWithJson(code int, obj any) error

	// ── Cookie ────────────────────────────────────────────────

	// Cookie 读取指定名称的 Cookie 值；不存在时返回空字符串。
	Cookie(name string) string

	// SetCookie 写入 Cookie；opts 控制过期时间、路径、HttpOnly 等。
	SetCookie(name, value string, opts CookieOptions)

	// ClearCookie 通过将 MaxAge 设为 -1 来删除指定 Cookie。
	ClearCookie(name string)
}

// ── 控制器接口 ────────────────────────────────────────────────────────

// Controller 控制器接口，所有控制器必须实现。
// 通过 Route.Register() 注册，框架自动处理 Prefix / Middleware 后调用 Boot。
type Controller interface {
	// Boot 声明该控制器的所有路由。
	// r 已被框架定位到正确的路由组下（根据 Prefixer / Middlewarer）。
	Boot(r Route)
}

// Prefixer 可选接口 —— 声明控制器路由前缀。
// Register() 检测到此接口时，自动为该控制器创建路由组。
type Prefixer interface {
	Prefix() string
}

// Middlewarer 可选接口 —— 声明控制器级中间件。
// Register() 检测到此接口时，自动为该控制器的路由组添加中间件。
type Middlewarer interface {
	Middleware() []HandlerFunc
}

// ── 路由服务契约 ──────────────────────────────────────────────────────

// Route HTTP 路由服务契约。
// 所有 Handler 使用 HandlerFunc 类型，应用代码无需引入任何底层 HTTP 框架。
type Route interface {
	// Run 启动 HTTP 服务器，addr 可选（默认读取配置）。
	Run(addr ...string) error
	// Shutdown 优雅关闭 HTTP 服务器。
	Shutdown() error

	// 路由注册方法，返回自身以支持链式调用。
	Get(path string, handler HandlerFunc) Route
	Post(path string, handler HandlerFunc) Route
	Put(path string, handler HandlerFunc) Route
	Delete(path string, handler HandlerFunc) Route
	Patch(path string, handler HandlerFunc) Route
	Head(path string, handler HandlerFunc) Route
	Options(path string, handler HandlerFunc) Route

	// Group 创建路由组（共享前缀和中间件）。
	// args 支持两种类型，可任意组合：
	//   - HandlerFunc  → 作为中间件
	//   - func(Route)  → 作为回调，在回调内声明该组的路由
	// 无回调时返回 group 对象供链式调用。
	Group(prefix string, args ...any) Route

	// Use 注册中间件，按注册顺序执行。
	Use(middleware ...HandlerFunc) Route

	// Register 批量注册控制器。
	// 框架自动检测控制器的 Prefix() 和 Middleware()（可选），
	// 然后调用 Boot(r) 让控制器声明自己的路由。
	Register(controllers ...Controller) Route

	// ── 静态资源 ───────────────────────────────────

	// Static 从本地目录 dir 提供静态文件服务，URL 前缀为 urlPrefix。
	// 例：Static("/static", "resources/static")
	Static(urlPrefix, dir string) Route

	// StaticFS 从任意 http.FileSystem 提供静态文件服务。
	// 配合 go:embed 使用：StaticFS("/static", http.FS(embeddedSubFS))
	StaticFS(urlPrefix string, fs http.FileSystem) Route

	// ── 高级特性 ───────────────────────────────────
	// Routes 获取所有路由。
	Routes() []RouteInfo
	// Route 获取指定路径的路由。
	Route(path string) RouteInfo
}

// CookieOptions 设置 Cookie 的选项。
type CookieOptions struct {
	// MaxAge 以秒为单位；0 表示会话 Cookie（浏览器关闭即失效），负数立即删除。
	MaxAge   int
	Path     string
	Domain   string
	Secure   bool
	HTTPOnly bool
	SameSite string // "Strict" | "Lax" | "None"
}

// Session 表示当前请求关联的会话对象。
// 实现方应保证并发安全。
type Session interface {
	// ID 返回当前会话 ID。
	ID() string

	// Get 读取会话中的值；不存在时返回 nil。
	Get(key string) any

	// GetString 读取字符串值，不存在时返回空字符串或默认值。
	GetString(key string, def ...string) string

	// GetInt 读取整型值。
	GetInt(key string, def ...int) int

	// GetBool 读取布尔值。
	GetBool(key string, def ...bool) bool

	// Set 设置会话值。
	Set(key string, value any)

	// Has 判断键是否存在。
	Has(key string) bool

	// Delete 删除会话中的键。
	Delete(key string)

	// Flash 设置一次性数据（下次读取后自动删除）。
	Flash(key string, value any)

	// GetFlash 读取一次性数据（读取后自动删除）。
	GetFlash(key string) any

	// Regenerate 重新生成会话 ID（防固定攻击）。
	Regenerate()

	// Destroy 销毁会话（清空所有数据并标记删除）。
	Destroy()

	// Save 将会话状态持久化到后端存储。由框架在响应发送前自动调用。
	Save() error
}

// SessionStore 会话存储后端接口。
type SessionStore interface {
	// New 根据 ID 加载已有会话；ID 为空时创建新会话。
	New(id string) (Session, error)

	// Delete 彻底删除指定 ID 的会话。
	Delete(id string) error

	// GC 清理过期会话（可选，驱动自行决定是否实现）。
	GC() error
}

// SessionManager 会话管理器。
type SessionManager interface {
	// Store 获取指定驱动的 SessionStore（如 "memory"、"cookie"）。
	Store(name ...string) SessionStore

	// Session 从当前请求上下文中获取已加载的 Session 对象。
	// 通常由中间件预先加载，控制器直接调用即可。
	Session(ctx Context) (Session, error)

	// SetCookieOptions 设置保存会话 ID 的 Cookie 选项。
	SetCookieOptions(opts CookieOptions)

	// CookieName 返回用于保存会话 ID 的 Cookie 名称。
	CookieName() string

	// Lifetime 返回会话有效期。
	Lifetime() time.Duration
}

// Validation 验证服务契约。
type Validation interface {
	// Validate 验证结构体，返回验证错误。
	Validate(obj any) error
	// RegisterRule 注册自定义验证规则。
	RegisterRule(rule any) error
}

// ViewEngine 是 HTML 模板渲染引擎契约。
// 默认实现位于 framework/http/view 包。
// 通过 HTTP ServiceProvider 自动注册（需在 config/config.yaml 中配置 view.dir）。
type ViewEngine interface {
	// AddFunc 注册单个自定义模板函数。
	// 调用后下次渲染时模板将被重新解析。
	AddFunc(name string, fn any) ViewEngine

	// AddFuncMap 批量注册自定义模板函数。
	// 调用后下次渲染时模板将被重新解析。
	AddFuncMap(fm template.FuncMap) ViewEngine

	// Load 从磁盘强制（重新）加载所有模板文件（线程安全）。
	// 惰性加载模式下，首次调用 Render 时会自动触发。
	Load() error

	// Render 将指定名称的模板与 data 合并后写入 w。
	// name 为相对于模板目录的路径，路径分隔符统一使用 "/"，例如 "home/index.html"。
	Render(w io.Writer, name string, data any) error
}

type RouteInfo struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}
