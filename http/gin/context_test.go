package gin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zhoudm1743/go-fast-framework/http/validation"
)

type bindReq struct {
	Name string `json:"name" binding:"required"`
}

func TestBindReturnsChineseError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// 模拟 NewRoute 中的初始化：禁用 gin 内置英文验证
	gin.DisableBindValidation()

	v, err := validation.NewValidator()
	if err != nil {
		t.Fatal(err)
	}

	body := bytes.NewBufferString(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", "application/json")

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	ctx := NewContext(c, v, nil, nil)

	var obj bindReq
	err = ctx.Bind(&obj)
	if err == nil {
		t.Fatal("期望验证失败")
	}
	if !strings.Contains(err.Error(), "必填") {
		t.Fatalf("期望中文错误消息，实际 %q", err.Error())
	}
}
