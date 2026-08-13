package base

import (
	"testing"
	"time"
)

type pageReq struct {
	Page int    `query:"page" default:"1"`
	Size int    `query:"size" default:"20"`
	Sort string `query:"sort" default:"desc"`
	Skip bool   `default:"-"`
}

type nestedReq struct {
	Inner innerReq `default:"-"`
}

type innerReq struct {
	Limit int `default:"10"`
}

type status int

func (s *status) UnmarshalText(text []byte) error {
	if string(text) == "active" {
		*s = 1
	}
	return nil
}

type fullReq struct {
	IDs     []int         `default:"1,2,3"`
	Tags    []string      `default:"a,b,c"`
	Timeout time.Duration `default:"5s"`
	Day     time.Time     `default:"2024-05-06"`
	Port    *int          `default:"8080"`
	Status  status        `default:"active"`
}

func TestApplyDefaults(t *testing.T) {
	req := pageReq{}
	ApplyDefaults(&req)

	if req.Page != 1 {
		t.Fatalf("期望 Page=1，实际 %d", req.Page)
	}
	if req.Size != 20 {
		t.Fatalf("期望 Size=20，实际 %d", req.Size)
	}
	if req.Sort != "desc" {
		t.Fatalf("期望 Sort=desc，实际 %q", req.Sort)
	}
}

func TestApplyDefaultsDoesNotOverride(t *testing.T) {
	req := pageReq{Page: 5, Size: 0}
	ApplyDefaults(&req)

	if req.Page != 5 {
		t.Fatalf("已有值不应被覆盖，期望 Page=5，实际 %d", req.Page)
	}
	if req.Size != 20 {
		t.Fatalf("零值应填默认，期望 Size=20，实际 %d", req.Size)
	}
}

func TestApplyDefaultsNested(t *testing.T) {
	req := nestedReq{}
	ApplyDefaults(&req)

	if req.Inner.Limit != 10 {
		t.Fatalf("嵌套结构体默认值未生效，期望 10，实际 %d", req.Inner.Limit)
	}
}

func TestApplyDefaultsAdvanced(t *testing.T) {
	req := fullReq{}
	ApplyDefaults(&req)

	if len(req.IDs) != 3 || req.IDs[0] != 1 || req.IDs[2] != 3 {
		t.Fatalf("切片默认值错误，实际 %v", req.IDs)
	}
	if len(req.Tags) != 3 || req.Tags[1] != "b" {
		t.Fatalf("字符串切片默认值错误，实际 %v", req.Tags)
	}
	if req.Timeout != 5*time.Second {
		t.Fatalf("Duration 默认值错误，实际 %v", req.Timeout)
	}
	if req.Day.Format(time.DateOnly) != "2024-05-06" {
		t.Fatalf("Time 默认值错误，实际 %v", req.Day)
	}
	if req.Port == nil || *req.Port != 8080 {
		t.Fatalf("指针默认值错误，实际 %v", req.Port)
	}
	if req.Status != 1 {
		t.Fatalf("自定义类型默认值错误，实际 %v", req.Status)
	}
}
