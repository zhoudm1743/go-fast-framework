package utils

import "math"

// PageUtil 分页工具集。
var PageUtil = pageUtil{}

type pageUtil struct{}

// PageMeta 分页元信息。
type PageMeta struct {
	Total   int64 `json:"total"`
	Page    int   `json:"page"`
	Size    int   `json:"size"`
	Pages   int   `json:"pages"`
	HasMore bool  `json:"has_more"`
}

func (r pageUtil) Normalize(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	return page, size
}

func (r pageUtil) Offset(page, size int) int {
	page, size = r.Normalize(page, size)
	return (page - 1) * size
}

func (r pageUtil) Meta(total int64, page, size int) PageMeta {
	page, size = r.Normalize(page, size)
	pages := 0
	if size > 0 {
		pages = int(math.Ceil(float64(total) / float64(size)))
	}
	return PageMeta{
		Total:   total,
		Page:    page,
		Size:    size,
		Pages:   pages,
		HasMore: page < pages,
	}
}
