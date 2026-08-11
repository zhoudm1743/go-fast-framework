package contracts

// ── 事件系统契约 ────────────────────────────────────────────────────────

// EventArg 事件参数，与 Goravel 保持兼容。
type EventArg struct {
	Type  string // 数据类型，如 "string"、"int"
	Value any    // 实际值
}

// EventQueue 监听器队列配置。
type EventQueue struct {
	Enable     bool   // 是否启用队列异步处理
	Connection string // 队列连接名
	Queue      string // 队列名
}

// Eventer 事件接口。
// Handle 用于加工/过滤事件参数，返回结果将传递给所有关联监听器。
type Eventer interface {
	Handle(args []EventArg) ([]EventArg, error)
}

// EventListener 事件监听器接口。
type EventListener interface {
	// Signature 监听器唯一标识。
	Signature() string
	// Queue 返回队列配置；Enable=false 时同步执行。
	Queue(args ...any) EventQueue
	// Handle 处理事件。
	Handle(args ...any) error
}

// EventPending 待派发事件对象（Builder 模式）。
type EventPending interface {
	Dispatch() error
}

// Event 事件总线服务契约（facades.Event() 返回此接口）。
type Event interface {
	// Register 注册事件 → 监听器映射（通常在引导阶段调用）。
	Register(events map[Eventer][]EventListener)
	// Job 创建待派发事件。
	Job(event Eventer, args []EventArg) EventPending
}
