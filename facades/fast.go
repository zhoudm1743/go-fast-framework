package facades

import "github.com/zhoudm1743/go-fast-framework/contracts"

// Fast 获取 Fast 控制台服务实例。
func Fast() contracts.Fast {
	return App().MustMake("fast").(contracts.Fast)
}
