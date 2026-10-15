package contracts

import "time"

// ── 全文检索契约 ────────────────────────────────────────────────────

// Search 全文检索服务契约（facades.Search() 返回此接口）。
// 驱动由外围仓库实现（如 gofast-meilisearch），本包只定义契约。
type Search interface {
	// Driver 切换到命名驱动；不传或空字符串为默认驱动（search.default）。
	Driver(name ...string) SearchDriver
}

// SearchDriver 检索驱动（单个引擎实例）。
type SearchDriver interface {
	// Index 获取索引操作对象。
	Index(name string) SearchIndex
}

// SearchIndex 索引操作。
type SearchIndex interface {
	// Upsert 新增或更新文档。文档为 map 或结构体；须含主键字段（驱动配置指定，默认 "id"）。
	Upsert(documents ...any) error
	// Search 检索索引。
	Search(query string, opts ...SearchOption) (*SearchResult, error)
	// Delete 按主键删除文档。
	Delete(ids ...any) error
	// Clear 清空索引全部文档。
	Clear() error
}

// SearchResult 检索结果。
type SearchResult struct {
	Hits  []map[string]any // 命中文档
	Total int64            // 命中总数（引擎支持时）
	Took  time.Duration    // 引擎耗时
}

// SearchOptions 检索选项集合。
type SearchOptions struct {
	Limit  int      // 返回条数上限
	Offset int      // 偏移
	Filter string   // 过滤表达式（引擎语法，如 meilisearch 的 filter）
	Sort   []string // 排序（如 ["price:asc"]）
}

// SearchOption 检索选项函数。
type SearchOption func(*SearchOptions)

// WithSearchLimit 限制返回条数。
func WithSearchLimit(n int) SearchOption {
	return func(o *SearchOptions) { o.Limit = n }
}

// WithSearchOffset 设置偏移。
func WithSearchOffset(n int) SearchOption {
	return func(o *SearchOptions) { o.Offset = n }
}

// WithSearchFilter 设置过滤表达式。
func WithSearchFilter(expr string) SearchOption {
	return func(o *SearchOptions) { o.Filter = expr }
}

// WithSearchSort 设置排序。
func WithSearchSort(fields ...string) SearchOption {
	return func(o *SearchOptions) { o.Sort = append(o.Sort, fields...) }
}
