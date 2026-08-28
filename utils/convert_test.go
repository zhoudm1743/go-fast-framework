package utils

import "testing"

func TestConvertUtilToInt(t *testing.T) {
	if ConvertUtil.ToInt("42") != 42 {
		t.Fatal("ToInt 失败")
	}
	if ConvertUtil.ToInt("", 9) != 9 {
		t.Fatal("ToInt 默认值失败")
	}
	if ConvertUtil.ToInt("x", 3) != 3 {
		t.Fatal("ToInt 解析失败应返回默认")
	}
}

func TestConvertUtilSlices(t *testing.T) {
	got := ConvertUtil.ToIntSlice("1,2,3")
	if len(got) != 3 || got[0] != 1 {
		t.Fatalf("ToIntSlice 失败: %v", got)
	}
}

func TestPtrUtil(t *testing.T) {
	p := PtrOf("hi")
	if PtrValue(p) != "hi" {
		t.Fatal("PtrValue 失败")
	}
	if PtrValueOr((*string)(nil), "def") != "def" {
		t.Fatal("PtrValueOr nil 失败")
	}
	if !PtrUtil.IsNil(nil) {
		t.Fatal("IsNil nil 应为 true")
	}
}

func TestHelperUtil(t *testing.T) {
	if HelperUtil.If(true, 1, 2) != 1 {
		t.Fatal("If 失败")
	}
	if !HelperUtil.Empty("") {
		t.Fatal("Empty 空串应为 true")
	}
	if HelperUtil.Default("", "x") != "x" {
		t.Fatal("Default 失败")
	}
}

func TestTimeUtilChain(t *testing.T) {
	u := TimeUtil.FromUnix(1700000000).StartOfDay().AddDays(1).Unix()
	if u <= 1700000000 {
		t.Fatalf("时间链失败: %d", u)
	}
}
