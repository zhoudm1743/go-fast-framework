package facades

import "github.com/zhoudm1743/go-fast-framework/contracts"

// Log 获取日志服务实例。
func Log() contracts.Log {
	return App().MustMake("log").(contracts.Log)
}
