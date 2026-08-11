package contracts

// ── 任务调度契约 ────────────────────────────────────────────────────────

// ScheduleEvent 调度事件接口（链式 Builder）。
type ScheduleEvent interface {
	// ── Cron 表达式 ─────────────────────────────────────────────
	// Cron 自定义 cron 表达式（支持 5 位分钟级和 6 位秒级）。
	Cron(expression string) ScheduleEvent

	// ── 秒级 ────────────────────────────────────────────────────
	EverySecond() ScheduleEvent
	EveryTwoSeconds() ScheduleEvent
	EveryFiveSeconds() ScheduleEvent
	EveryTenSeconds() ScheduleEvent
	EveryFifteenSeconds() ScheduleEvent
	EveryTwentySeconds() ScheduleEvent
	EveryThirtySeconds() ScheduleEvent

	// ── 分钟级 ──────────────────────────────────────────────────
	EveryMinute() ScheduleEvent
	EveryTwoMinutes() ScheduleEvent
	EveryThreeMinutes() ScheduleEvent
	EveryFourMinutes() ScheduleEvent
	EveryFiveMinutes() ScheduleEvent
	EveryTenMinutes() ScheduleEvent
	EveryFifteenMinutes() ScheduleEvent
	EveryThirtyMinutes() ScheduleEvent

	// ── 小时级 ──────────────────────────────────────────────────
	Hourly() ScheduleEvent
	HourlyAt(minute int) ScheduleEvent
	EveryTwoHours() ScheduleEvent
	EveryThreeHours() ScheduleEvent
	EveryFourHours() ScheduleEvent
	EverySixHours() ScheduleEvent

	// ── 日级 ────────────────────────────────────────────────────
	Daily() ScheduleEvent
	DailyAt(t string) ScheduleEvent

	// ── 周级 ────────────────────────────────────────────────────
	Days(days ...int) ScheduleEvent // 1=Monday … 7=Sunday
	Weekdays() ScheduleEvent
	Weekends() ScheduleEvent
	Mondays() ScheduleEvent
	Tuesdays() ScheduleEvent
	Wednesdays() ScheduleEvent
	Thursdays() ScheduleEvent
	Fridays() ScheduleEvent
	Saturdays() ScheduleEvent
	Sundays() ScheduleEvent
	Weekly() ScheduleEvent

	// ── 月/季/年级 ──────────────────────────────────────────────
	Monthly() ScheduleEvent
	Quarterly() ScheduleEvent
	Yearly() ScheduleEvent

	// ── 并发控制 ────────────────────────────────────────────────
	// SkipIfStillRunning 如果上次执行未完成，则跳过本次。
	SkipIfStillRunning() ScheduleEvent
	// DelayIfStillRunning 如果上次执行未完成，等待后再执行。
	DelayIfStillRunning() ScheduleEvent

	// ── 分布式 ──────────────────────────────────────────────────
	// OnOneServer 使用缓存锁确保任务只在一台服务器上运行。
	OnOneServer() ScheduleEvent

	// ── 标识 ────────────────────────────────────────────────────
	// Name 为任务命名（OnOneServer 必须时使用）。
	Name(name string) ScheduleEvent

	// GetName 返回任务名称。
	GetName() string
	// GetCron 返回 cron 表达式。
	GetCron() string
}

// Schedule 调度器服务契约（facades.Schedule() 返回此接口）。
type Schedule interface {
	// Call 注册闭包调度任务。
	Call(callback func()) ScheduleEvent
	// Command 注册 Fast 命令调度任务。
	Command(command string) ScheduleEvent
}
