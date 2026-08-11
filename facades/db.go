package facades

import "github.com/zhoudm1743/go-fast-framework/contracts"

// DB 获取数据库管理器（推荐使用）。
func DB() contracts.DB {
	return App().MustMake("db").(contracts.DB)
}
