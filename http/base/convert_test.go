package base

import (
	"reflect"
	"testing"
)

type customInt int
type customUint uint
type customFloat float64
type customBool bool

type convertTarget struct {
	I  int
	UI customUint
	F  customFloat
	B  customBool
	S  string
}

func TestSetFieldFromString(t *testing.T) {
	var target convertTarget
	rv := reflect.ValueOf(&target).Elem()

	SetFieldFromString(rv.FieldByName("I"), "42")
	if target.I != 42 {
		t.Fatalf("int 期望 42，实际 %d", target.I)
	}

	SetFieldFromString(rv.FieldByName("UI"), "7")
	if target.UI != 7 {
		t.Fatalf("custom uint 期望 7，实际 %d", target.UI)
	}

	SetFieldFromString(rv.FieldByName("F"), "3.14")
	if float64(target.F) != 3.14 {
		t.Fatalf("custom float 期望 3.14，实际 %v", target.F)
	}

	SetFieldFromString(rv.FieldByName("B"), "true")
	if bool(target.B) != true {
		t.Fatalf("custom bool 期望 true，实际 %v", target.B)
	}

	SetFieldFromString(rv.FieldByName("S"), " hello ")
	if target.S != " hello " {
		t.Fatalf("string 不应 Trim，实际 %q", target.S)
	}

	SetFieldFromString(rv.FieldByName("I"), " 99 ")
	if target.I != 99 {
		t.Fatalf("int 带空格期望 Trim 后 99，实际 %d", target.I)
	}

	SetFieldFromString(rv.FieldByName("I"), "abc")
	if target.I != 99 {
		t.Fatalf("解析失败不应覆盖已有值，实际 %d", target.I)
	}
}
