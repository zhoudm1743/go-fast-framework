package contracts

import "time"

// ── 队列系统契约 ────────────────────────────────────────────────────────

// QueueArg 队列任务参数。
type QueueArg struct {
	Type  string // 数据类型，如 "string"、"int"
	Value any    // 实际值
}

// QueueJob 队列任务接口。
type QueueJob interface {
	// Signature 任务唯一标识。
	Signature() string
	// Handle 执行任务。
	Handle(args ...any) error
}

// QueueChain 任务链条目。
type QueueChain struct {
	Job  QueueJob
	Args []QueueArg
}

// QueuePending 待派发任务（Builder 模式）。
type QueuePending interface {
	// OnQueue 指定队列名。
	OnQueue(queue string) QueuePending
	// OnConnection 指定连接名。
	OnConnection(connection string) QueuePending
	// Delay 延迟到指定时间后处理。
	Delay(delay time.Time) QueuePending
	// Retry 失败后的最大重试次数（不含首次执行；默认 0 表示不重试）。
	Retry(times int) QueuePending
	// Dispatch 推送到队列异步执行。
	Dispatch() error
	// DispatchSync 同步立即执行（不走队列）。
	DispatchSync() error
}

// Queue 队列服务契约（facades.Queue() 返回此接口）。
type Queue interface {
	// Job 创建单个任务的待派发对象。
	Job(job QueueJob, args []QueueArg) QueuePending
	// Chain 创建链式任务的待派发对象。
	Chain(jobs []QueueChain) QueuePending
}
