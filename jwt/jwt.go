package jwt

import (
	"errors"
	"fmt"
	"sync"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
	"github.com/zhoudm1743/go-fast-framework/contracts"
)

type jwtService struct {
	secret     []byte
	ttl        time.Duration // token 有效期
	signingAlg gojwt.SigningMethod
}

var _ contracts.JWTDriver = (*jwtService)(nil)

// newFromConfig 从 cfg 的指定前缀读取 guard 配置并创建 jwtService。
// prefix 示例："jwt"（默认 guard）或 "jwt.guards.platform"（命名 guard）。
func newFromConfig(cfg contracts.Config, prefix string) (*jwtService, error) {
	secret := cfg.GetString(prefix+".secret", "")
	if secret == "" {
		return nil, errors.New("jwt: secret is required, please set " + prefix + ".secret in config")
	}

	ttlMin := cfg.GetInt(prefix+".ttl", 60)
	algName := cfg.GetString(prefix+".alg", "HS256")

	var alg gojwt.SigningMethod
	switch algName {
	case "HS384":
		alg = gojwt.SigningMethodHS384
	case "HS512":
		alg = gojwt.SigningMethodHS512
	default:
		alg = gojwt.SigningMethodHS256
	}

	return &jwtService{
		secret:     []byte(secret),
		ttl:        time.Duration(ttlMin) * time.Minute,
		signingAlg: alg,
	}, nil
}

// GenerateToken 根据 MapClaims 生成签名 Token，自动注入 exp 字段。
func (j *jwtService) GenerateToken(claims gojwt.MapClaims) (string, error) {
	// 自动设置过期时间（调用方传入的 exp 会被覆盖）
	claims["exp"] = gojwt.NewNumericDate(time.Now().Add(j.ttl))
	if _, ok := claims["iat"]; !ok {
		claims["iat"] = gojwt.NewNumericDate(time.Now())
	}

	token := gojwt.NewWithClaims(j.signingAlg, claims)
	return token.SignedString(j.secret)
}

// ParseToken 解析并验证 Token，返回 MapClaims。
func (j *jwtService) ParseToken(tokenStr string) (gojwt.MapClaims, error) {
	token, err := gojwt.ParseWithClaims(
		tokenStr,
		gojwt.MapClaims{},
		func(t *gojwt.Token) (any, error) {
			if t.Method.Alg() != j.signingAlg.Alg() {
				return nil, errors.New("jwt: unexpected signing algorithm")
			}
			return j.secret, nil
		},
	)
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("jwt: token is invalid")
	}

	claims, ok := token.Claims.(gojwt.MapClaims)
	if !ok {
		return nil, errors.New("jwt: failed to parse claims")
	}
	return claims, nil
}

// RefreshToken 在 Token 仍有效时刷新过期时间，返回新 Token。
func (j *jwtService) RefreshToken(tokenStr string) (string, error) {
	claims, err := j.ParseToken(tokenStr)
	if err != nil {
		return "", err
	}

	// 删除旧时间戳，由 GenerateToken 重新注入
	delete(claims, "exp")
	delete(claims, "iat")
	return j.GenerateToken(claims)
}

// ── jwtManager：默认 guard + 命名 guard 调度 ─────────────────────────

// jwtManager 实现 contracts.JWT，负责默认 guard 与命名 guard 的分发。
// 默认 guard 使用顶层 jwt.* 配置；命名 guard 使用 jwt.guards.<name>.* 配置，
// 首次访问时懒加载并缓存。
type jwtManager struct {
	cfg        contracts.Config
	defaultJWT contracts.JWTDriver
	guards     map[string]contracts.JWTDriver
	mu         sync.RWMutex
}

var _ contracts.JWT = (*jwtManager)(nil)

// New 创建 JWT 管理器。默认 guard 使用顶层 jwt.* 配置；
// 命名 guard 使用 jwt.guards.<name>.* 配置，首次访问时懒加载并缓存。
//
// 读取配置键：
//
//	jwt.secret  — 默认 guard 签名密钥（必填）
//	jwt.ttl     — 默认 guard 有效期（分钟），默认 60
//	jwt.alg     — 默认 guard 签名算法，支持 HS256 / HS384 / HS512，默认 HS256
//	jwt.guards.<name>.secret — 命名 guard 签名密钥（必填）
//	jwt.guards.<name>.ttl    — 命名 guard 有效期（分钟），默认 60
//	jwt.guards.<name>.alg    — 命名 guard 签名算法，默认 HS256
func New(cfg contracts.Config) (contracts.JWT, error) {
	def, err := newFromConfig(cfg, "jwt")
	if err != nil {
		return nil, err
	}
	return &jwtManager{
		cfg:        cfg,
		defaultJWT: def,
		guards:     make(map[string]contracts.JWTDriver),
	}, nil
}

// Guard 返回指定 guard 的 JWTDriver 实例；空名返回默认 guard。
// guard 未在配置中声明时 panic。
func (m *jwtManager) Guard(name string) contracts.JWTDriver {
	if name == "" {
		return m.defaultJWT
	}

	// 快速路径：已缓存
	m.mu.RLock()
	g, ok := m.guards[name]
	m.mu.RUnlock()
	if ok {
		return g
	}

	// 慢路径：double-checked locking
	m.mu.Lock()
	defer m.mu.Unlock()
	if g, ok = m.guards[name]; ok {
		return g
	}

	// guard 未在配置中声明
	if m.cfg.Get("jwt.guards."+name) == nil {
		panic(fmt.Sprintf("[GoFast] jwt guard %q not found in config", name))
	}

	var err error
	g, err = newFromConfig(m.cfg, "jwt.guards."+name)
	if err != nil {
		panic(fmt.Sprintf("[GoFast] jwt guard %q init failed: %v", name, err))
	}
	m.guards[name] = g
	return g
}

func (m *jwtManager) GenerateToken(claims gojwt.MapClaims) (string, error) {
	return m.defaultJWT.GenerateToken(claims)
}

func (m *jwtManager) ParseToken(tokenStr string) (gojwt.MapClaims, error) {
	return m.defaultJWT.ParseToken(tokenStr)
}

func (m *jwtManager) RefreshToken(tokenStr string) (string, error) {
	return m.defaultJWT.RefreshToken(tokenStr)
}
