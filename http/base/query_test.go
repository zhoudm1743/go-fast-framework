package base

import "testing"

func TestQueryInt(t *testing.T) {
	if v := QueryInt("42"); v != 42 {
		t.Fatalf("期望 42，实际 %d", v)
	}
	if v := QueryInt(""); v != 0 {
		t.Fatalf("空值期望 0，实际 %d", v)
	}
	if v := QueryInt("", 7); v != 7 {
		t.Fatalf("空值期望默认 7，实际 %d", v)
	}
	if v := QueryInt("abc", 7); v != 7 {
		t.Fatalf("解析失败期望默认 7，实际 %d", v)
	}
	if v := QueryInt(" 42 "); v != 42 {
		t.Fatalf("带空格期望 Trim 后 42，实际 %d", v)
	}
}

func TestQueryInt64(t *testing.T) {
	if v := QueryInt64("9223372036854775807"); v != 9223372036854775807 {
		t.Fatalf("期望 int64 最大值，实际 %d", v)
	}
	if v := QueryInt64("", 9); v != 9 {
		t.Fatalf("空值期望默认 9，实际 %d", v)
	}
}

func TestQueryFloat64(t *testing.T) {
	if v := QueryFloat64("3.14"); v != 3.14 {
		t.Fatalf("期望 3.14，实际 %v", v)
	}
	if v := QueryFloat64("", 2.5); v != 2.5 {
		t.Fatalf("空值期望默认 2.5，实际 %v", v)
	}
}

func TestQueryBool(t *testing.T) {
	if v := QueryBool("true"); v != true {
		t.Fatalf("期望 true，实际 %v", v)
	}
	if v := QueryBool("1"); v != true {
		t.Fatalf("'1' 期望 true，实际 %v", v)
	}
	if v := QueryBool("", true); v != true {
		t.Fatalf("空值期望默认 true，实际 %v", v)
	}
}
