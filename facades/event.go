package facades

import "github.com/zhoudm1743/go-fast-framework/contracts"

// Event 获取事件总线实例。
func Event() contracts.Event {
	return app.MustMake("event").(contracts.Event)
}
