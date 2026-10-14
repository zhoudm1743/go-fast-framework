package facades

import "github.com/zhoudm1743/go-fast-framework/contracts"

// Notify 获取消息通知服务实例。
func Notify() contracts.Notify {
	return App().MustMake("notify").(contracts.Notify)
}
