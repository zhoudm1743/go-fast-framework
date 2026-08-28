package base

import (
	"testing"

	"github.com/zhoudm1743/go-fast-framework/utils"
)

func TestResponsePaginateNormalize(t *testing.T) {
	page, size := utils.PageUtil.Normalize(0, 0)
	if page != 1 || size != 20 {
		t.Fatalf("期望 page=1 size=20，实际 page=%d size=%d", page, size)
	}
	page, size = utils.PageUtil.Normalize(-1, -5)
	if page != 1 || size != 20 {
		t.Fatalf("负数期望 page=1 size=20，实际 page=%d size=%d", page, size)
	}
}
