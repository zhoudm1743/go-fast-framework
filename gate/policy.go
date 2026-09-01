package gate

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// policyResolver 负责解析并调用 Policy 方法。
type policyResolver struct {
	mu       sync.RWMutex
	policies map[reflect.Type]contracts.Policy
}

func newPolicyResolver() *policyResolver {
	return &policyResolver{
		policies: make(map[reflect.Type]contracts.Policy),
	}
}

func (pr *policyResolver) register(model any, policy contracts.Policy) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	typ := reflect.TypeOf(model)
	pr.policies[typ] = policy
}

// resolve 尝试根据 ability 与参数找到匹配的 Policy 方法并调用。
// 返回 (response, found)。found=false 表示没有匹配的策略。
func (pr *policyResolver) resolve(ctx contracts.Context, ability string, args []any) (contracts.GateResponse, bool) {
	if len(args) == 0 {
		return nil, false
	}

	// 查找第一个非空参数作为模型
	var model any
	var modelType reflect.Type
	for _, arg := range args {
		if arg == nil {
			continue
		}
		model = arg
		modelType = reflect.TypeOf(arg)
		break
	}
	if model == nil {
		return nil, false
	}

	pr.mu.RLock()
	policy, ok := pr.policies[modelType]
	pr.mu.RUnlock()
	if !ok {
		return nil, false
	}

	// 能力名转方法名：update → Update，update-post → UpdatePost
	methodName := abilityToMethodName(ability)
	method := reflect.ValueOf(policy).MethodByName(methodName)
	if !method.IsValid() {
		// 回退尝试首字母大写
		method = reflect.ValueOf(policy).MethodByName(strings.ToUpper(ability[:1]) + ability[1:])
	}
	if !method.IsValid() {
		return nil, false
	}

	res := callPolicyMethod(method, ctx, model)
	return res, true
}

func abilityToMethodName(ability string) string {
	parts := strings.Split(ability, "-")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}

func callPolicyMethod(method reflect.Value, ctx contracts.Context, model any) contracts.GateResponse {
	defer func() {
		if r := recover(); r != nil {
			_ = r
		}
	}()

	mt := method.Type()
	in := make([]reflect.Value, 0, mt.NumIn())

	for i := 0; i < mt.NumIn(); i++ {
		paramType := mt.In(i)
		if i == 0 && paramType.Implements(reflect.TypeOf((*contracts.Context)(nil)).Elem()) {
			in = append(in, reflect.ValueOf(ctx))
			continue
		}
		modelVal := reflect.ValueOf(model)
		if modelVal.Type().ConvertibleTo(paramType) {
			in = append(in, modelVal.Convert(paramType))
		} else if modelVal.Type().AssignableTo(paramType) {
			in = append(in, modelVal)
		} else {
			return newResponse(false, fmt.Sprintf("policy method parameter mismatch: expected %v, got %v", paramType, modelVal.Type()))
		}
	}

	out := method.Call(in)
	if len(out) == 0 {
		return newResponse(false, "policy method returned no value")
	}

	// 支持返回 bool 或 GateResponse
	first := out[0]
	switch v := first.Interface().(type) {
	case bool:
		return newResponse(v, "")
	case contracts.GateResponse:
		return v
	case error:
		if v != nil {
			return newResponse(false, v.Error())
		}
		return newResponse(true, "")
	default:
		return newResponse(false, "policy method returned unsupported type")
	}
}
