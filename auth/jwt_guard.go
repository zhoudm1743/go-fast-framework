package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
	"github.com/zhoudm1743/go-fast-framework/contracts"
	fid "github.com/zhoudm1743/go-fast-framework/id"
	"golang.org/x/crypto/bcrypt"
)

// jwtGuard JWT 认证守卫：token 管道完全委托现有 contracts.JWT，
// 本层只增加认证语义（用户加载、黑名单、ctx 集成）。
type jwtGuard struct {
	name     string
	jwt      contracts.JWTDriver
	provider contracts.UserProvider
	cache    contracts.CacheStore
}

// newJWTGuard 创建 JWT 守卫。
func newJWTGuard(name string, jwtDriver contracts.JWTDriver, provider contracts.UserProvider, cache contracts.CacheStore) *jwtGuard {
	return &jwtGuard{name: name, jwt: jwtDriver, provider: provider, cache: cache}
}

func (g *jwtGuard) Login(ctx contracts.Context, user contracts.AuthUser) (string, error) {
	return g.LoginUsingID(ctx, user.GetAuthID())
}

func (g *jwtGuard) LoginUsingID(_ contracts.Context, id any) (string, error) {
	return g.jwt.GenerateToken(gojwt.MapClaims{
		"sub":   id,
		"guard": g.name,
		"jti":   fid.New(), // 每次签发唯一，保证同一用户一秒内的多个 token 互不串黑名单
	})
}

func (g *jwtGuard) Attempt(ctx contracts.Context, identity, password string) (string, error) {
	user, err := g.provider.RetrieveByIdentity(ctx, identity)
	if err != nil {
		return "", ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(user.GetAuthPassword()), []byte(password)) != nil {
		return "", ErrInvalidCredentials
	}
	return g.Login(ctx, user)
}

// Parse 解析凭证字符串（可带 Bearer 前缀），返回载荷。
func (g *jwtGuard) Parse(_ contracts.Context, token string) (*contracts.AuthPayload, error) {
	token = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(token), "Bearer "))
	if token == "" {
		return nil, ErrUnauthenticated
	}
	if g.revoked(token) {
		return nil, ErrTokenRevoked
	}
	claims, err := g.jwt.ParseToken(token)
	if err != nil {
		if errors.Is(err, gojwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, fmt.Errorf("auth: 凭证解析失败: %w", err)
	}
	return &contracts.AuthPayload{
		Guard:    g.name,
		ID:       claims["sub"],
		IssuedAt: claimTime(claims["iat"]),
		ExpireAt: claimTime(claims["exp"]),
	}, nil
}

func (g *jwtGuard) User(ctx contracts.Context) (contracts.AuthUser, error) {
	if v := ctx.Value(string(contracts.CtxKeyAuthUser)); v != nil {
		if user, ok := v.(contracts.AuthUser); ok {
			return user, nil
		}
	}
	payload, err := g.Parse(ctx, ctx.Header("Authorization"))
	if err != nil {
		return nil, err
	}
	user, err := g.provider.RetrieveByID(ctx, payload.ID)
	if err != nil {
		return nil, ErrUnauthenticated
	}
	return user, nil
}

func (g *jwtGuard) ID(ctx contracts.Context) (any, error) {
	if v := ctx.Value(string(contracts.CtxKeyUserID)); v != nil {
		return v, nil
	}
	if v := ctx.Value(string(contracts.CtxKeyAuthUser)); v != nil {
		if user, ok := v.(contracts.AuthUser); ok {
			return user.GetAuthID(), nil
		}
	}
	payload, err := g.Parse(ctx, ctx.Header("Authorization"))
	if err != nil {
		return nil, err
	}
	return payload.ID, nil
}

func (g *jwtGuard) Check(ctx contracts.Context) bool {
	_, err := g.ID(ctx)
	return err == nil
}

// Refresh 刷新当前请求的凭证；旧凭证进黑名单（防重放）。
// 直接重建 claims 并轮换 jti（jwt.RefreshToken 不轮换，秒内刷新会产生相同 token）。
func (g *jwtGuard) Refresh(ctx contracts.Context) (string, error) {
	token := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(ctx.Header("Authorization")), "Bearer "))
	payload, err := g.Parse(ctx, token) // 黑名单中的旧凭证不允许刷新
	if err != nil {
		return "", err
	}
	claims, err := g.jwt.ParseToken(token)
	if err != nil {
		return "", fmt.Errorf("auth: 凭证刷新失败: %w", err)
	}
	delete(claims, "exp")
	delete(claims, "iat")
	claims["jti"] = fid.New()
	newToken, err := g.jwt.GenerateToken(claims)
	if err != nil {
		return "", fmt.Errorf("auth: 凭证刷新失败: %w", err)
	}
	g.blacklist(token, payload.ExpireAt)
	return newToken, nil
}

// Logout 登出：当前凭证进黑名单至其自然过期。无凭证视为幂等成功。
func (g *jwtGuard) Logout(ctx contracts.Context) error {
	token := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(ctx.Header("Authorization")), "Bearer "))
	if token == "" {
		return nil
	}
	claims, err := g.jwt.ParseToken(token)
	if err != nil {
		return fmt.Errorf("auth: 登出失败: %w", err)
	}
	g.blacklist(token, claimTime(claims["exp"]))
	return nil
}

// revoked 检查凭证是否在黑名单中。
func (g *jwtGuard) revoked(token string) bool {
	if g.cache == nil {
		return false
	}
	return g.cache.Has(blacklistKey(token))
}

// blacklist 将凭证加入黑名单，TTL 至其自然过期。
func (g *jwtGuard) blacklist(token string, expireAt time.Time) {
	if g.cache == nil {
		return
	}
	ttl := time.Until(expireAt)
	if ttl <= 0 {
		ttl = time.Minute // 已过期 token 无需长期占用缓存
	}
	_ = g.cache.Put(blacklistKey(token), 1, ttl)
}

// blacklistKey 黑名单缓存键（token 哈希，避免超长键与明文凭证落缓存）。
func blacklistKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return "auth:jwt:blacklist:" + hex.EncodeToString(sum[:])
}

// claimTime 解析 JWT 时间 claim（JSON 数值往返为 float64 秒时间戳）。
func claimTime(v any) time.Time {
	switch t := v.(type) {
	case float64:
		return time.Unix(int64(t), 0)
	case int64:
		return time.Unix(t, 0)
	case gojwt.NumericDate:
		return t.Time
	case *gojwt.NumericDate:
		return t.Time
	}
	return time.Time{}
}
