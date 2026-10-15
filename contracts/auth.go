package contracts

import "time"

// ── 认证服务契约 ────────────────────────────────────────────────────

// Auth 认证服务契约（facades.Auth() 返回此接口）。
// 采用 Guard（守卫，认证方式）+ UserProvider（用户提供者，用户加载）双扩展模型。
type Auth interface {
	// Guard 切换到命名守卫；不传或空字符串为默认守卫（auth.default_guard）。
	Guard(name ...string) Guard
	// Extend 注册自定义守卫驱动；重复注册同名驱动返回错误。
	Extend(name string, factory GuardFactory) error
	// Provider 注册用户提供者；重复注册同名提供者返回错误。
	Provider(name string, p UserProvider) error
}

// Guard 认证守卫契约：凭证的签发、解析与当前用户解析。
type Guard interface {
	// Login 为用户签发凭证。
	Login(ctx Context, user AuthUser) (string, error)
	// LoginUsingID 按用户 ID 签发凭证。
	LoginUsingID(ctx Context, id any) (string, error)
	// Attempt 身份 + 密码登录；校验失败返回 auth.ErrInvalidCredentials。
	Attempt(ctx Context, identity, password string) (string, error)
	// Parse 解析凭证字符串（可带 Bearer 前缀），返回载荷。
	Parse(ctx Context, token string) (*AuthPayload, error)
	// User 返回当前请求的已认证用户：
	// 优先取 ctx 中已注入的用户（认证中间件），否则解析 Authorization 头并加载。
	User(ctx Context) (AuthUser, error)
	// ID 返回当前请求的用户 ID。
	ID(ctx Context) (any, error)
	// Check 当前请求是否已认证。
	Check(ctx Context) bool
	// Refresh 刷新当前请求的凭证；旧凭证作废。
	Refresh(ctx Context) (string, error)
	// Logout 登出：当前凭证作废。
	Logout(ctx Context) error
}

// AuthPayload 凭证载荷。
type AuthPayload struct {
	Guard    string    // 签发该凭证的守卫名
	ID       any       // 用户标识
	IssuedAt time.Time // 签发时间
	ExpireAt time.Time // 过期时间
}

// AuthUser 可认证用户契约。业务用户模型实现以下两个方法即可被 auth 加载。
type AuthUser interface {
	// GetAuthID 返回用户唯一标识（写入凭证的 sub）。
	GetAuthID() any
	// GetAuthPassword 返回密码的 bcrypt hash（用于 Attempt 校验）。
	GetAuthPassword() string
}

// UserProvider 用户提供者：从存储加载用户。
type UserProvider interface {
	// RetrieveByID 按 ID 加载用户；查无返回 auth.ErrUserNotFound（包装）。
	RetrieveByID(ctx Context, id any) (AuthUser, error)
	// RetrieveByIdentity 按登录身份（用户名/邮箱/手机号…）加载用户；查无返回 auth.ErrUserNotFound（包装）。
	RetrieveByIdentity(ctx Context, identity string) (AuthUser, error)
}

// GuardFactory 守卫驱动工厂。
// name 为当前守卫名；驱动实现所需的运行期依赖（jwt/cache 等）应在注册时通过闭包捕获。
type GuardFactory func(name string, cfg map[string]any, provider UserProvider) (Guard, error)
