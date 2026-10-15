# auth 认证服务设计

日期：2026-10-15
状态：已确认，实现中
模块：`go-fast-framework`

参考：[Goravel 用户认证](https://www.goravel.dev/zh_CN/security/authentication.html) 的 Guard/UserProvider 双扩展模型，适配本框架现有件。

## 1. 背景与目标

框架已有 `jwt`（多 Guard token 签发/解析/刷新）、`gate`（授权，从 ctx 取当前用户）、`http/session`、gorm 审计回调（读 `CtxKeyUserID` 填充 created_by/updated_by），但缺少"认证"层：解析凭证 → 加载当前用户 → 注入 ctx。

目标：新增 `auth` 服务，补齐这一层。**不重做 JWT 管道**，jwt guard 内部完全委托现有 `contracts.JWT`。

## 2. 关键决策

| 决策点 | 结论 |
|---|---|
| 模型 | Goravel 式 Guard + UserProvider 双注册表（`Extend` / `Provider`） |
| v1 Guard | 只做 **jwt guard**；session guard 留扩展位 |
| JWT 底层 | 复用 `contracts.JWT`（`facades.Http.JWT()`），guard 名映射 `jwt.guards.<name>`，缺省用默认 jwt |
| 用户模型 | 业务结构体实现 `contracts.AuthUser` 接口；`User()` 返回接口，**不做反射填充** |
| 密码 | bcrypt（`golang.org/x/crypto` 已在依赖，零新增） |
| 登出 | token 哈希进 cache 黑名单，TTL=剩余有效期 |
| Refresh | 刷新后旧 token 进黑名单（Laravel 语义，防重放） |
| ctx 集成 | 中间件写 `CtxKeyUserID`（gate/审计自动生效）+ `CtxKeyAuthUser`（用户对象） |
| 明确不做 | session guard、remember me、邮箱验证、反射填充业务模型 |

## 3. 目录结构

```
go-fast-framework/
  contracts/auth.go            # Auth / Guard / AuthUser / UserProvider / AuthPayload / 哨兵错误
  contracts/context_keys.go    # 追加 CtxKeyAuthUser
  auth/
    service_provider.go        # 注册 "auth" 单例 + jwt guard 内置驱动
    auth.go                    # authManager：Guard 调度、Extend/Provider 注册表
    jwt_guard.go               # jwt 守卫（登录/解析/刷新/登出/黑名单）
    db_provider.go             # NewDBUserProvider：基于 contracts.Query 的用户加载
    auth_test.go
    jwt_guard_test.go
    db_provider_test.go
  http/middleware/auth.go      # Auth(guard...) 中间件：401 / 写 ctx
  facades/auth.go              # facades.Auth()
```

## 4. 契约 API（contracts/auth.go）

```go
// Auth 认证服务契约（facades.Auth() 返回此接口）。
type Auth interface {
    // Guard 切换到命名守卫；不传或空字符串为默认守卫（auth.default_guard）。
    Guard(name ...string) Guard
    // Extend 注册自定义守卫驱动（重复名报错）。
    Extend(name string, factory GuardFactory) error
    // Provider 注册用户提供者（重复名报错）。
    Provider(name string, p UserProvider) error
}

type Guard interface {
    // Login 为用户签发凭证。
    Login(ctx Context, user AuthUser) (string, error)
    // LoginUsingID 按用户 ID 签发凭证。
    LoginUsingID(ctx Context, id any) (string, error)
    // Attempt 身份 + 密码登录；校验失败返回 ErrInvalidCredentials。
    Attempt(ctx Context, identity, password string) (string, error)
    // Parse 解析凭证字符串（可带 Bearer 前缀），返回载荷。
    Parse(ctx Context, token string) (*AuthPayload, error)
    // User 返回当前请求的已认证用户：
    // 优先取 ctx 中已注入的用户（中间件），否则解析 Authorization 头并加载。
    User(ctx Context) (AuthUser, error)
    // ID 返回当前请求的用户 ID。
    ID(ctx Context) (any, error)
    // Check 当前请求是否已认证。
    Check(ctx Context) bool
    // Refresh 刷新当前请求的凭证；旧凭证进黑名单。
    Refresh(ctx Context) (string, error)
    // Logout 登出：当前凭证进黑名单至其自然过期。
    Logout(ctx Context) error
}

// AuthPayload 凭证载荷。
type AuthPayload struct {
    Guard    string
    ID       any
    IssuedAt time.Time
    ExpireAt time.Time
}

// AuthUser 可认证用户契约。业务用户模型实现以下两个方法即可。
type AuthUser interface {
    GetAuthID() any
    GetAuthPassword() string // bcrypt hash
}

// UserProvider 用户提供者。
type UserProvider interface {
    RetrieveByID(ctx Context, id any) (AuthUser, error)
    RetrieveByIdentity(ctx Context, identity string) (AuthUser, error)
}

// GuardFactory 守卫驱动工厂。
type GuardFactory func(app foundation.Application, guardName string, cfg map[string]any, provider UserProvider) (Guard, error)
```

哨兵错误（auth 包）：`ErrUnauthenticated` / `ErrInvalidCredentials` / `ErrTokenExpired` / `ErrTokenRevoked` / `ErrUserNotFound`。jwt 解析错误中 `errors.Is(err, gojwt.ErrTokenExpired)` 映射为 `ErrTokenExpired`。

⚠️ `GuardFactory` 引用 `foundation.Application` 会造成 contracts → foundation 反向依赖（foundation 已 import contracts）。因此工厂签名中的 app 参数改为 ** contracts 层无 foundation 依赖的形态**：

```go
type GuardFactory func(cfg map[string]any, provider UserProvider) (Guard, error)
```

jwt guard 的 jwt/cache 依赖在工厂内部闭包捕获（内置驱动在 auth 包注册时闭包持有 app）。

## 5. jwt guard 语义

- claims：`sub`（用户 ID，原值）、`guard`（守卫名）、`jti`（每次签发唯一，`id.New()`，防止同一秒内同用户 token 相同导致黑名单串杀；Refresh 时轮换）。`exp`/`iat` 由 jwt 服务自动注入。
- 数值 ID 经 JSON 往返后为 float64；db provider 按值查询不受影响（记录在案，不改）。
- 底层 jwt 守卫选择：`auth.guards.<name>.jwt_guard` 显式指定 > `jwt.guards.<name>` 存在则用同名 > 默认 jwt。
- 黑名单 key：`auth:jwt:blacklist:<sha256(token)>`，值 1，TTL = exp - now（已过期 token 兜底 1 分钟）。命中黑名单 → `ErrTokenRevoked`。
- `User(ctx)`：先查 ctx 中 `CtxKeyAuthUser`；未命中则解析 `Authorization` 头（Bearer 前缀可选），再 `RetrieveByID` 加载。**透传 `ErrTokenExpired`/`ErrTokenRevoked`**（客户端可据此刷新）；未携带凭证或查无用户 → `ErrUnauthenticated`。
- `Refresh(ctx)`：直接重建 claims（删除 exp/iat、轮换 jti）重新签发，不依赖 `jwt.RefreshToken`（其不轮换 jti，秒内刷新会产生相同 token）；旧凭证进黑名单。
- `Attempt(ctx, identity, password)`：`RetrieveByIdentity` + `bcrypt.CompareHashAndPassword`；用户不存在或密码不符统一返回 `ErrInvalidCredentials`（不泄露用户存在性）。

## 6. db user provider

```go
auth.NewDBUserProvider(auth.DBUserProviderConfig{
    Table:           "users",
    IDColumn:        "id",                    // 默认 id
    IdentityColumns: []string{"username"},    // OR 匹配
}, func() contracts.AuthUser { return &models.User{} })
```

- `RetrieveByID`：`db.Query().Table(t).Where(idCol+" = ?", id).First(factory())`；查无/查询失败 → `ErrUserNotFound`（包装原错误）。
- `RetrieveByIdentity`：`Where("(col1 = ? OR col2 = ?)", ...)` 同理。

## 7. 配置

```yaml
auth:
  default_guard: api
  guards:
    api:
      driver: jwt
      provider: users
      # jwt_guard: api   # 可选，缺省规则见 §5
    admin:
      driver: jwt
      provider: admins
```

`providers` 不经 YAML 声明（模型类型必须在代码中给），业务侧在自定义 ServiceProvider 的 Boot 中调用 `facades.Auth().Provider("users", auth.NewDBUserProvider(...))`。

ConfigProvider 默认值：`auth.default_guard = "api"`。

## 8. 中间件（http/middleware/auth.go）

```go
// Auth 认证中间件：解析凭证、加载用户、写入 ctx；失败 401。
func Auth(guard ...string) contracts.HandlerFunc
```

成功路径写 `CtxKeyUserID`（gate 授权与 gorm 审计自动生效）与 `CtxKeyAuthUser`（用户对象，`Guard.User()` 直接命中，免重复解析）。失败 `ctx.Response().Unauthorized()`。

## 9. 错误处理

| 场景 | 返回 |
|---|---|
| 无/空凭证 | ErrUnauthenticated |
| token 过期 | ErrTokenExpired |
| token 在黑名单 | ErrTokenRevoked |
| 签名/格式错误 | 包装原 jwt 错误 |
| Attempt 用户不存在或密码错误 | ErrInvalidCredentials |
| provider 查无用户 | ErrUserNotFound（包装） |

## 10. 测试方案（全部无外部依赖）

- **jwt guard**：真实 jwt 服务（内存配置）+ mock cache + stub http ctx（嵌入 `contracts.Context` 接口，仅实现 Header/Value/WithValue）
  - Login→Parse 往返、Bearer 前缀兼容、过期 → ErrTokenExpired、登出后 → ErrTokenRevoked、Refresh 后旧 token 失效
  - `User()`：中间件注入优先、header 解析路径、未认证
- **db provider**：`mock.MockQuery`（TableFunc/WhereFunc/FirstFunc 链式记录）；查无 → ErrUserNotFound；identity 多列 OR 条件
- **Attempt**：bcrypt 成功/失败两路径，统一 ErrInvalidCredentials
- **manager**：Guard 切换/默认、Extend/Provider 重复名报错、jwt_guard 映射规则
- **中间件**：stub ctx + mock，验证 401、ctx 注入两个键
- **集成**：foundation app + config/log/cache/jwt/auth provider 装配，facades.Auth() 端到端

## 11. 验收标准

1. `go build ./...` / `go vet ./...` / `go test ./...` 全绿
2. 业务方按 README 最小接入：模型实现 AuthUser + 注册 provider + 配置 guard + 挂中间件，即可完成登录签发/受保护路由
3. gate 授权与 gorm 审计在认证请求中自动拿到用户 ID
4. 零新增第三方依赖
