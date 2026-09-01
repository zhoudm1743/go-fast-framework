package contracts

// Gate 授权服务契约，提供 Laravel 风格的 Gate / Policy 能力。
type Gate interface {
	// Define 注册一个命名能力（ability）。
	// callback 接收当前 Context 与任意参数，返回是否允许。
	Define(ability string, callback GateCallback)
	// Policy 为模型类型注册策略。
	Policy(model any, policy Policy)
	// Before 注册前置回调，所有能力检查前都会执行；返回 false 可直接拒绝。
	Before(callback GateBeforeCallback)
	// After 注册后置回调，用于记录审计等。
	After(callback GateAfterCallback)

	// Allows 检查是否允许指定能力。
	Allows(ctx Context, ability string, args ...any) bool
	// Denies 检查是否拒绝指定能力。
	Denies(ctx Context, ability string, args ...any) bool
	// Inspect 返回详细的授权结果。
	Inspect(ctx Context, ability string, args ...any) GateResponse
	// ForUser 为指定用户构造一个检查上下文（不传则使用 ctx 中的当前用户）。
	ForUser(user any) GateUser
}

// GateCallback Gate 能力回调。
type GateCallback func(ctx Context, args ...any) bool

// GateBeforeCallback Gate 前置回调。
type GateBeforeCallback func(ctx Context, ability string, args ...any) GateResponse

// GateAfterCallback Gate 后置回调。
type GateAfterCallback func(ctx Context, ability string, response GateResponse, args ...any)

// GateResponse 授权检查结果。
type GateResponse interface {
	// Allowed 是否允许。
	Allowed() bool
	// Denied 是否拒绝。
	Denied() bool
	// Message 拒绝原因（允许时为空）。
	Message() string
}

// GateUser 指定用户后的授权检查入口。
type GateUser interface {
	// Allows 检查该用户是否允许指定能力。
	Allows(ability string, args ...any) bool
	// Denies 检查该用户是否拒绝指定能力。
	Denies(ability string, args ...any) bool
}

// Policy 模型策略接口。
// 具体策略应实现 View / ViewAny / Create / Update / Delete / Restore / ForceDelete 等方法。
// 方法签名约定：func (p *Policy) Action(ctx Context, model *Model) bool
// 框架通过反射自动匹配模型类型与方法名。
type Policy interface{}
