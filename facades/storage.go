package facades

import "github.com/zhoudm1743/go-fast-framework/contracts"

// Storage 获取文件存储服务实例。
func Storage() contracts.Storage {
	return App().MustMake("storage").(contracts.Storage)
}
