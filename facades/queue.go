package facades

import "github.com/zhoudm1743/go-fast-framework/contracts"

// Queue 获取队列服务实例。
func Queue() contracts.Queue {
	return app.MustMake("queue").(contracts.Queue)
}
