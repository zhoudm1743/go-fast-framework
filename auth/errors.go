package auth

import "errors"

// 认证服务的哨兵错误。
var (
	// ErrUnauthenticated 未认证（未携带凭证或无法解析当前用户）。
	ErrUnauthenticated = errors.New("auth: 未认证")
	// ErrInvalidCredentials 身份或密码错误（Attempt 统一返回，不泄露用户存在性）。
	ErrInvalidCredentials = errors.New("auth: 身份或密码错误")
	// ErrTokenExpired 凭证已过期。
	ErrTokenExpired = errors.New("auth: token 已过期")
	// ErrTokenRevoked 凭证已注销（登出或刷新后的旧 token）。
	ErrTokenRevoked = errors.New("auth: token 已注销")
	// ErrUserNotFound 用户提供者查无用户。
	ErrUserNotFound = errors.New("auth: 用户不存在")
)
