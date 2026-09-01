package gate

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// gate 实现 contracts.Gate。
type gate struct {
	mu         sync.RWMutex
	abilities  map[string]contracts.GateCallback
	before     []contracts.GateBeforeCallback
	after      []contracts.GateAfterCallback
	policies   *policyResolver
}

// NewGate 创建 Gate 服务实例。
func NewGate() contracts.Gate {
	return &gate{
		abilities: make(map[string]contracts.GateCallback),
		policies:  newPolicyResolver(),
	}
}

func (g *gate) Define(ability string, callback contracts.GateCallback) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.abilities[ability] = callback
}

func (g *gate) Policy(model any, policy contracts.Policy) {
	g.policies.register(model, policy)
}

func (g *gate) Before(callback contracts.GateBeforeCallback) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.before = append(g.before, callback)
}

func (g *gate) After(callback contracts.GateAfterCallback) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.after = append(g.after, callback)
}

func (g *gate) Allows(ctx contracts.Context, ability string, args ...any) bool {
	return g.Inspect(ctx, ability, args...).Allowed()
}

func (g *gate) Denies(ctx contracts.Context, ability string, args ...any) bool {
	return g.Inspect(ctx, ability, args...).Denied()
}

func (g *gate) Inspect(ctx contracts.Context, ability string, args ...any) contracts.GateResponse {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// 1. Before 回调
	for _, before := range g.before {
		res := before(ctx, ability, args...)
		if res != nil && res.Denied() {
			g.runAfter(ctx, ability, res, args)
			return res
		}
	}

	// 2. Gate 能力定义
	if callback, ok := g.abilities[ability]; ok {
		res := newResponse(callback(ctx, args...), "")
		g.runAfter(ctx, ability, res, args)
		return res
	}

	// 3. Policy 自动匹配
	if res, ok := g.policies.resolve(ctx, ability, args); ok {
		g.runAfter(ctx, ability, res, args)
		return res
	}

	// 4. 未找到任何定义，默认拒绝
	res := newResponse(false, fmt.Sprintf("ability %q not defined", ability))
	g.runAfter(ctx, ability, res, args)
	return res
}

func (g *gate) ForUser(user any) contracts.GateUser {
	return &gateUser{gate: g, user: user}
}

func (g *gate) runAfter(ctx contracts.Context, ability string, response contracts.GateResponse, args []any) {
	for _, after := range g.after {
		after(ctx, ability, response, args...)
	}
}

// gateUser 实现 contracts.GateUser。
type gateUser struct {
	gate *gate
	user any
}

func (gu *gateUser) Allows(ability string, args ...any) bool {
	// 构造一个最小 Context 只包含 user
	ctx := &userContext{user: gu.user}
	return gu.gate.Allows(ctx, ability, args...)
}

func (gu *gateUser) Denies(ability string, args ...any) bool {
	return !gu.Allows(ability, args...)
}

// userContext 是一个仅用于 GateUser 的最小 Context 包装。
type userContext struct {
	user any
}

func (c *userContext) User() any { return c.user }

// 这些空实现仅为了满足 contracts.Context 接口；GateUser 场景不会调用 HTTP 相关方法。
func (c *userContext) Method() string                                 { return "" }
func (c *userContext) Path() string                                   { return "" }
func (c *userContext) Param(key string) string                        { return "" }
func (c *userContext) Query(key string, defaultValue ...string) string { return "" }
func (c *userContext) QueryInt(key string, defaultValue ...int) int     { return 0 }
func (c *userContext) QueryInt64(key string, defaultValue ...int64) int64 { return 0 }
func (c *userContext) QueryFloat64(key string, defaultValue ...float64) float64 { return 0 }
func (c *userContext) QueryBool(key string, defaultValue ...bool) bool { return false }
func (c *userContext) Header(key string) string                        { return "" }
func (c *userContext) IP() string                                      { return "" }
func (c *userContext) BodyRaw() []byte                                 { return nil }
func (c *userContext) FormValue(key string) string                     { return "" }
func (c *userContext) ContentType() string                             { return "" }
func (c *userContext) UserAgent() string                               { return "" }
func (c *userContext) FullPath() string                                { return "" }
func (c *userContext) Bind(obj any) error                              { return nil }
func (c *userContext) File(key string) (contracts.File, error)         { return nil, nil }
func (c *userContext) Files(key string) ([]contracts.File, error)      { return nil, nil }
func (c *userContext) Storage() contracts.Storage                      { return nil }
func (c *userContext) SendFile(path string) error                      { return nil }
func (c *userContext) JSON(code int, obj any) error                    { return nil }
func (c *userContext) String(code int, s string) error                 { return nil }
func (c *userContext) Redirect(code int, location string) error        { return nil }
func (c *userContext) HTML(code int, name string, data any) error      { return nil }
func (c *userContext) Response() contracts.Response                    { return nil }
func (c *userContext) Status(code int) contracts.Context               { return c }
func (c *userContext) SetHeader(key, value string) contracts.Context   { return c }
func (c *userContext) Write(data []byte) error                         { return nil }
func (c *userContext) Value(key string) any                            { return nil }
func (c *userContext) WithValue(key string, value any) contracts.Context { return c }
func (c *userContext) Next() error                                     { return nil }
func (c *userContext) Abort() error                                    { return nil }
func (c *userContext) AbortWithCode(code int) error                    { return nil }
func (c *userContext) AbortWithJson(code int, obj any) error           { return nil }
func (c *userContext) Cookie(name string) string                       { return "" }
func (c *userContext) SetCookie(name, value string, opts contracts.CookieOptions) {}
func (c *userContext) ClearCookie(name string)                           {}

// ensure userContext implements contracts.Context
var _ contracts.Context = (*userContext)(nil)

// Helper to detect if arg is a model type.
func isModelType(t reflect.Type) bool {
	return t.Kind() == reflect.Ptr || t.Kind() == reflect.Struct
}
