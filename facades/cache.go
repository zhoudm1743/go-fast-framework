package facades

import "github.com/zhoudm1743/go-fast-framework/contracts"

// Cache 获取缓存服务实例。
func Cache() contracts.Cache {
	return App().MustMake("cache").(contracts.Cache)
}
