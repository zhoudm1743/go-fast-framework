package facades

import "github.com/zhoudm1743/go-fast-framework/contracts"

// Auth 获取认证服务实例。
func Auth() contracts.Auth {
	return App().MustMake("auth").(contracts.Auth)
}
