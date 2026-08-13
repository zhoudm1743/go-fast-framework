package validation

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// validatorImpl 实现 contracts.Validation 接口。
type validatorImpl struct {
	validate *validator.Validate
	// messages 记录自定义规则名 → 错误消息模板，用于验证失败时替换默认消息。
	messages map[string]string
}

// NewValidator 创建验证器实例。
func NewValidator() (*validatorImpl, error) {
	v := validator.New()

	v.SetTagName("binding")

	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		for _, tagKey := range []string{"json", "form", "query"} {
			if tag := fld.Tag.Get(tagKey); tag != "" {
				name := strings.SplitN(tag, ",", 2)[0]
				if name != "" && name != "-" {
					return name
				}
			}
		}
		return fld.Name
	})

	return &validatorImpl{validate: v, messages: make(map[string]string)}, nil
}

func (v *validatorImpl) Validate(obj any) error {
	if obj == nil {
		return nil
	}
	val := reflect.ValueOf(obj)
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return nil
		}
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return nil
	}
	if err := v.validate.Struct(obj); err != nil {
		return v.formatError(err)
	}
	return nil
}

// RegisterRule 注册自定义验证规则。
func (v *validatorImpl) RegisterRule(rule contracts.ValidationRule) error {
	if rule == nil {
		return fmt.Errorf("验证规则不能为 nil")
	}
	name := rule.Rule()
	if name == "" {
		return fmt.Errorf("验证规则名称不能为空")
	}

	if err := v.validate.RegisterValidation(name, func(fl validator.FieldLevel) bool {
		return rule.Validate(fieldValue(fl), fl.Param())
	}); err != nil {
		return fmt.Errorf("注册验证规则 %q 失败: %w", name, err)
	}

	if msg := rule.Message(); msg != "" {
		v.messages[name] = msg
	}
	return nil
}

// fieldValue 安全地取出字段值，nil 指针返回 nil，避免 Interface() panic。
func fieldValue(fl validator.FieldLevel) any {
	f := fl.Field()
	if !f.IsValid() {
		return nil
	}
	if f.Kind() == reflect.Ptr && f.IsNil() {
		return nil
	}
	return f.Interface()
}

// formatError 将验证错误转换为友好消息：自定义规则使用其 Message 模板，
// 其余规则保留 go-playground 默认文本。
func (v *validatorImpl) formatError(err error) error {
	var verr validator.ValidationErrors
	if !errors.As(err, &verr) {
		return err
	}

	msgs := make([]string, 0, len(verr))
	for _, fe := range verr {
		if msg, ok := v.messages[fe.Tag()]; ok {
			msgs = append(msgs, strings.ReplaceAll(msg, ":attribute", fe.Field()))
			continue
		}
		msgs = append(msgs, fe.Error())
	}
	return errors.New(strings.Join(msgs, "; "))
}
