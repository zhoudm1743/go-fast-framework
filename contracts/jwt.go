package contracts

import "github.com/golang-jwt/jwt/v5"

// JWTDriver 单个 JWT 守卫（Guard）的服务能力：签发、解析、刷新。
// 每个 Guard 拥有独立的密钥（secret）、有效期（ttl）和签名算法（alg）。
type JWTDriver interface {
	// GenerateToken 根据 MapClaims 生成签名 Token 字符串。
	// claims 中可包含任意业务字段，框架层会自动注入 exp（过期时间）。
	GenerateToken(claims jwt.MapClaims) (string, error)

	// ParseToken 解析并验证 Token，返回 MapClaims。
	// Token 无效、过期或签名错误时返回 error。
	ParseToken(tokenStr string) (jwt.MapClaims, error)

	// RefreshToken 在 Token 仍有效时刷新过期时间，返回新 Token。
	RefreshToken(tokenStr string) (string, error)
}

// JWT JWT 服务契约，嵌入 JWTDriver 使默认 Guard 的方法可直接调用，
// 同时支持通过 Guard(name) 切换到命名 Guard（读取 jwt.guards.<name>.* 配置）。
//
// 使用示例：
//
//	// 默认 Guard
//	token, _ := facades.Http.JWT().GenerateToken(claims)
//
//	// 命名 Guard（例如 platform 平台）
//	claims, _ := facades.Http.JWT().Guard("platform").ParseToken(token)
type JWT interface {
	JWTDriver

	// Guard 返回指定名称的 JWT 守卫实例。
	// name 为空时返回默认 Guard；guard 未在配置中声明时 panic。
	Guard(name string) JWTDriver
}
