package validation

import (
	"strings"
	"testing"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

type phoneRule struct{}

func (r *phoneRule) Rule() string { return "phone" }
func (r *phoneRule) Validate(field contracts.ValidationField) bool {
	s, ok := field.Value().(string)
	return ok && len(s) == 11
}
func (r *phoneRule) Message() string { return ":attribute 格式不正确" }

type userReq struct {
	Phone string `json:"phone" binding:"required,phone"`
}

func TestRegisterRuleAndMessage(t *testing.T) {
	v, err := NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	if err := v.RegisterRule(&phoneRule{}); err != nil {
		t.Fatal(err)
	}

	if err := v.Validate(&userReq{Phone: "13800138000"}); err != nil {
		t.Fatalf("期望验证通过，实际 %v", err)
	}

	err = v.Validate(&userReq{Phone: "123"})
	if err == nil {
		t.Fatal("期望验证失败")
	}
	if !strings.Contains(err.Error(), "格式不正确") {
		t.Fatalf("期望包含自定义消息，实际 %q", err.Error())
	}
	if !strings.Contains(err.Error(), "phone") {
		t.Fatalf("期望 :attribute 被替换为字段名，实际 %q", err.Error())
	}
}

// confirmRule 跨字段比较规则：确认密码需与密码一致。
type confirmRule struct{}

func (r *confirmRule) Rule() string { return "confirm" }
func (r *confirmRule) Validate(field contracts.ValidationField) bool {
	other, ok := field.Field("password")
	return ok && field.Value() == other
}
func (r *confirmRule) Message() string { return ":attribute 与密码不一致" }

type registerReq struct {
	Password string `json:"password" binding:"required"`
	Confirm  string `json:"confirm" binding:"required,confirm"`
}

func TestCrossFieldRule(t *testing.T) {
	v, err := NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	if err := v.RegisterRule(&confirmRule{}); err != nil {
		t.Fatal(err)
	}

	// 一致 → 通过
	if err := v.Validate(&registerReq{Password: "abc123", Confirm: "abc123"}); err != nil {
		t.Fatalf("期望验证通过，实际 %v", err)
	}

	// 不一致 → 失败且带自定义消息
	err = v.Validate(&registerReq{Password: "abc123", Confirm: "different"})
	if err == nil {
		t.Fatal("期望验证失败")
	}
	if !strings.Contains(err.Error(), "与密码不一致") {
		t.Fatalf("期望跨字段自定义消息，实际 %q", err.Error())
	}
}

func TestRegisterNilRule(t *testing.T) {
	v, _ := NewValidator()
	if err := v.RegisterRule(nil); err == nil {
		t.Fatal("期望 nil 规则报错")
	}
}

type emptyNameRule struct{}

func (r *emptyNameRule) Rule() string                                  { return "" }
func (r *emptyNameRule) Validate(field contracts.ValidationField) bool { return true }
func (r *emptyNameRule) Message() string                               { return "" }

func TestRegisterEmptyRuleName(t *testing.T) {
	v, _ := NewValidator()
	if err := v.RegisterRule(&emptyNameRule{}); err == nil {
		t.Fatal("期望空规则名报错")
	}
}
