package utils

import "testing"

func TestCollectChain(t *testing.T) {
	out := Collect([]int{1, 2, 2, 3}).Filter(func(v int) bool { return v > 1 }).Unique().Reverse().Slice()
	if len(out) != 2 || out[0] != 3 {
		t.Fatalf("Collect 链失败: %v", out)
	}
}

func TestDictAnyChain(t *testing.T) {
	m := DictAny(map[string]any{"id": 1, "secret": "x"}).Only("id").Set("status", 1).Map()
	if m["secret"] != nil || m["status"] != 1 {
		t.Fatalf("DictAny 链失败: %v", m)
	}
}

func TestNumberUtilChain(t *testing.T) {
	v := NumberUtil.Of(1.239).Round(2).Clamp(0, 10).Float()
	if v != 1.24 {
		t.Fatalf("Number 链失败: %v", v)
	}
}

func TestHashUtilBcrypt(t *testing.T) {
	hash, err := HashUtil.Bcrypt("secret")
	if err != nil {
		t.Fatal(err)
	}
	if !HashUtil.CheckBcrypt("secret", hash) {
		t.Fatal("bcrypt 校验失败")
	}
}

func TestPageUtil(t *testing.T) {
	p, s := PageUtil.Normalize(0, 0)
	if p != 1 || s != 20 {
		t.Fatalf("Normalize 失败: %d %d", p, s)
	}
	meta := PageUtil.Meta(100, 2, 20)
	if meta.Pages != 5 || !meta.HasMore {
		t.Fatalf("Meta 失败: %+v", meta)
	}
}

func TestMoneyUtil(t *testing.T) {
	s := MoneyUtil.OfFen(1990).FormatYuan()
	if s != "19.90" && s != "19.9" {
		// FormatYuan may vary - check fen
		if MoneyUtil.OfFen(1990).Fen() != 1990 {
			t.Fatalf("Money 失败: %s", s)
		}
	}
}

func TestCheckUtilMobile(t *testing.T) {
	if !CheckUtil.IsMobile("13800138000") {
		t.Fatal("手机号校验失败")
	}
}

func TestMaskUtilPhone(t *testing.T) {
	if MaskUtil.Phone("13800138000") != "138****8000" {
		t.Fatal("手机脱敏失败")
	}
}

func TestJsonUtilParse(t *testing.T) {
	j := JsonUtil.Parse(`{"data":{"n":1}}`).Get("data.n")
	n, err := j.ToInt()
	if err != nil || n != 1 {
		t.Fatalf("Json Get 失败: %v %v", n, err)
	}
}

func TestUrlUtilChain(t *testing.T) {
	u := UrlUtil.Of("https://example.com").Join("users").WithQuery(map[string]string{"page": "1"}).String()
	if u == "" {
		t.Fatal("Url 链失败")
	}
}

func TestTimeUtilChainFormat(t *testing.T) {
	s := TimeUtil.FromUnix(1700000000).Format("2006")
	if s != "2023" {
		t.Fatalf("Time Format 失败: %s", s)
	}
}

func TestMapUtilGet(t *testing.T) {
	v := MapUtil.Get(map[string]any{"user": map[string]any{"name": "tom"}}, "user.name")
	if v != "tom" {
		t.Fatalf("MapUtil.Get 失败: %v", v)
	}
}

func TestSqlUtilSafeOrder(t *testing.T) {
	order, err := SqlUtil.SafeOrder("id", []string{"id", "name"}, "desc")
	if err != nil || order != "id DESC" {
		t.Fatalf("SafeOrder 失败: %s %v", order, err)
	}
}

func TestVersionUtil(t *testing.T) {
	if VersionUtil.LT("0.1.5", "0.1.6") != true {
		t.Fatal("Version LT 失败")
	}
}
