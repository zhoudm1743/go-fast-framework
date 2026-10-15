package facades

import "github.com/zhoudm1743/go-fast-framework/contracts"

// Search 获取全文检索服务实例（需注册驱动 Provider，如 gofast-meilisearch）。
func Search() contracts.Search {
	return App().MustMake("search").(contracts.Search)
}
