package utils

import "testing"

func ptrOf[T any](v T) *T { return &v }

type updateReq struct {
	Code    *string `json:"code"`
	Name    *string `json:"name"`
	Age     *int    `json:"age"`
	Active  *bool   `json:"active"`
	NoTag   *string
	Ignored *string `json:"-"`
	skip    *string
}

func TestStructUtilPtrToMap(t *testing.T) {
	req := &updateReq{
		Code:    ptrOf("C001"),
		Name:    ptrOf("tom"),
		NoTag:   ptrOf("hello"),
		Ignored: ptrOf("x"),
	}

	m := StructUtil.PtrToMap(req)

	if m["code"] != "C001" {
		t.Fatalf("期望 code=C001，实际 %v", m["code"])
	}
	if m["name"] != "tom" {
		t.Fatalf("期望 name=tom，实际 %v", m["name"])
	}
	if m["no_tag"] != "hello" {
		t.Fatalf("无 tag 字段应转 snake_case，期望 no_tag=hello，实际 %v", m["no_tag"])
	}
	if _, ok := m["age"]; ok {
		t.Fatalf("nil 指针字段不应出现在 map 中，实际 %v", m)
	}
	if _, ok := m["ignored"]; ok {
		t.Fatalf("json:\"-\" 字段不应出现在 map 中，实际 %v", m)
	}
	if _, ok := m["skip"]; ok {
		t.Fatalf("未导出字段不应出现在 map 中，实际 %v", m)
	}
}

type innerReq struct {
	Remark *string `json:"remark"`
}

type nestedReq struct {
	Code   *string   `json:"code"`
	Inner  innerReq  `json:"inner"`
	Inner2 *innerReq `json:"inner2"`
}

func TestStructUtilPtrToMapNested(t *testing.T) {
	req := &nestedReq{
		Code:   ptrOf("C002"),
		Inner:  innerReq{Remark: ptrOf("r1")},
		Inner2: &innerReq{Remark: ptrOf("r2")},
	}

	m := StructUtil.PtrToMap(req)

	if m["code"] != "C002" {
		t.Fatalf("期望 code=C002，实际 %v", m["code"])
	}
	if m["remark"] != "r2" {
		t.Fatalf("嵌套指针结构体应摊平，期望 remark=r2，实际 %v", m["remark"])
	}
}

func TestStructUtilPtrToMapZeroValuePointer(t *testing.T) {
	req := &updateReq{
		Age:    ptrOf(0),
		Active: ptrOf(false),
		Code:   ptrOf(""),
	}

	m := StructUtil.PtrToMap(req)

	if v, ok := m["age"]; !ok || v != 0 {
		t.Fatalf("*int 指向 0 应保留在 map 中，实际 %v", m)
	}
	if v, ok := m["active"]; !ok || v != false {
		t.Fatalf("*bool 指向 false 应保留在 map 中，实际 %v", m)
	}
	if v, ok := m["code"]; !ok || v != "" {
		t.Fatalf("*string 指向空串应保留在 map 中，实际 %v", m)
	}
}

func TestStructUtilPtrToMapNonStruct(t *testing.T) {
	m := StructUtil.PtrToMap(123)
	if len(m) != 0 {
		t.Fatalf("非结构体应返回空 map，实际 %v", m)
	}

	var req *updateReq
	m = StructUtil.PtrToMap(req)
	if len(m) != 0 {
		t.Fatalf("nil 指针应返回空 map，实际 %v", m)
	}
}

type fullStruct struct {
	Code string  `json:"code"`
	Name string  `json:"name"`
	Age  int     `json:"age"`
	Note *string `json:"note"`
}

func TestStructUtilStructToMap(t *testing.T) {
	note := "hi"
	s := fullStruct{Code: "C1", Age: 18, Note: &note}

	m := StructUtil.StructToMap(&s)

	if m["code"] != "C1" {
		t.Fatalf("期望 code=C1，实际 %v", m["code"])
	}
	if m["age"] != 18 {
		t.Fatalf("期望 age=18，实际 %v", m["age"])
	}
	if m["note"] != "hi" {
		t.Fatalf("期望 note=hi，实际 %v", m["note"])
	}
	if _, ok := m["name"]; ok {
		t.Fatalf("零值字段不应出现，实际 %v", m)
	}
}

type srcDTO struct {
	Code string
	Age  int
	Name string
}

type dstEntity struct {
	Code  string
	Age   int64
	Name  string
	Extra string
}

func TestStructUtilCopy(t *testing.T) {
	src := srcDTO{Code: "C", Age: 30, Name: "n"}
	var dst dstEntity

	if err := StructUtil.Copy(&dst, src); err != nil {
		t.Fatalf("Copy 失败: %v", err)
	}

	if dst.Code != "C" || dst.Name != "n" {
		t.Fatalf("同名字段未拷贝，实际 %+v", dst)
	}
	if dst.Age != 30 {
		t.Fatalf("int→int64 应自动转换，实际 %d", dst.Age)
	}
	if dst.Extra != "" {
		t.Fatalf("无对应字段应保持零值，实际 %q", dst.Extra)
	}
}

func TestStructUtilCopyError(t *testing.T) {
	if err := StructUtil.Copy(struct{}{}, srcDTO{}); err == nil {
		t.Fatal("非指针 dst 应报错")
	}
	var dst dstEntity
	if err := StructUtil.Copy(&dst, 123); err == nil {
		t.Fatal("非结构体 src 应报错")
	}
}

func TestStructUtilIsZero(t *testing.T) {
	if !StructUtil.IsZero(nil) {
		t.Fatal("nil 应视为零值")
	}
	if !StructUtil.IsZero("") {
		t.Fatal("空字符串应视为零值")
	}
	if !StructUtil.IsZero(0) {
		t.Fatal("0 应视为零值")
	}
	if StructUtil.IsZero("x") {
		t.Fatal("非空字符串不应视为零值")
	}
	if !StructUtil.IsZero(fullStruct{}) {
		t.Fatal("零值结构体应视为零值")
	}
}
