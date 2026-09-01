package facades

import "github.com/zhoudm1743/go-fast-framework/contracts"

// Mock 获取 Mock 管理服务实例。
func Mock() contracts.MockManager {
	return app.MustMake("mock").(contracts.MockManager)
}
