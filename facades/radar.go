package facades

import "github.com/zhoudm1743/go-fast-framework/contracts"

// Radar 获取可观测性服务实例。
func Radar() contracts.Radar {
	return App().MustMake("radar").(contracts.Radar)
}
