package facades

import "github.com/zhoudm1743/go-fast-framework/contracts"

// Gate 获取授权服务实例。
func Gate() contracts.Gate {
	return app.MustMake("gate").(contracts.Gate)
}
