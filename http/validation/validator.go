package validation

import (
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

// validatorImpl 实现 contracts.Validation 接口。
type validatorImpl struct {
	validate *validator.Validate
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

	return &validatorImpl{validate: v}, nil
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
	return v.validate.Struct(obj)
}

func (v *validatorImpl) RegisterRule(rule any) error {
	type registrationFuncer interface {
		Rule() string
		RegistrationFunc() validator.Func
	}
	if r, ok := rule.(registrationFuncer); ok {
		return v.validate.RegisterValidation(r.Rule(), r.RegistrationFunc())
	}
	return nil
}
