package facades

import "github.com/zhoudm1743/go-fast-framework/contracts"

// Process 获取进程管理服务实例。
func Process() contracts.Process {
	return app.MustMake("process").(contracts.Process)
}
