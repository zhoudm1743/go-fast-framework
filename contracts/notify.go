package contracts

import "time"

// ── 消息通知契约 ────────────────────────────────────────────────────

// Notify 消息通知服务契约（facades.Notify() 返回此接口）。
// 统一的发送门面：调用方构造消息、自选通道；同步 Send 或异步 Dispatch。
type Notify interface {
	// Mail 构造邮件消息（SMTP 通道）。
	Mail() MailMessage
	// Webhook 构造 Webhook 消息（HTTP POST 到指定 URL）。
	Webhook(url string) WebhookMessage
	// Log 获取日志通道（消息写入框架日志，供开发调试）。
	Log() LogNotify
}

// MailMessage 邮件消息构造器。
type MailMessage interface {
	// To 添加收件人。
	To(addrs ...string) MailMessage
	// Subject 设置主题。
	Subject(s string) MailMessage
	// Text 设置纯文本正文。
	Text(body string) MailMessage
	// HTML 设置 HTML 正文；与 Text 同设时发送 multipart/alternative。
	HTML(body string) MailMessage
	// From 覆盖配置中的默认发件人。
	From(addr string) MailMessage
	// Send 同步发送。
	Send() error
	// Dispatch 异步发送：消息进入队列，由队列 worker 执行。
	Dispatch(opts ...DispatchOption) error
}

// WebhookMessage Webhook 消息构造器。
type WebhookMessage interface {
	// Header 设置请求头。
	Header(k, v string) WebhookMessage
	// JSON 设置 JSON 正文（自动设置 Content-Type: application/json）。
	JSON(v any) WebhookMessage
	// Body 设置原始正文（Content-Type 用 Header 另指定）；与 JSON 后设者覆盖先设者。
	Body(b []byte) WebhookMessage
	// IdempotencyKey 设置幂等键（Idempotency-Key 请求头）。
	// 不设置则首次发送前自动生成（框架 id 包）；所有重试复用同一键。
	IdempotencyKey(key string) WebhookMessage
	// Retry 同步 Send 的内置重试次数（默认取配置 notify.webhook.retry）。
	// 仅影响同步路径；Dispatch 的重试由 WithRetry 交给队列。
	Retry(times int) WebhookMessage
	// RetryInterval 设置同步重试的退避基数，指数增长。
	RetryInterval(d time.Duration) WebhookMessage
	// Send 同步发送（含内置幂等重试）。
	Send() error
	// Dispatch 异步发送：消息进入队列，由队列 worker 执行（不走内置重试，幂等键随 payload 传递）。
	Dispatch(opts ...DispatchOption) error
}

// LogNotify 日志通道：消息写入框架日志服务，供开发调试。
type LogNotify interface {
	Debug(args ...any)
	Info(args ...any)
	Warn(args ...any)
	Error(args ...any)
	// WithField 添加单个字段，返回新的 LogNotify 实例（链式调用）。
	WithField(key string, value any) LogNotify
	// WithFields 添加多个字段，返回新的 LogNotify 实例（链式调用）。
	WithFields(fields map[string]any) LogNotify
	// WithError 添加 error 字段。
	WithError(err error) LogNotify
}

// DispatchSettings Dispatch 选项集合。
type DispatchSettings struct {
	Queue string    // 队列名，空为默认队列
	Delay time.Time // 延迟到指定时间后处理
	Retry int       // 队列侧最大重试次数（不含首次执行）
}

// DispatchOption Dispatch 配置函数。
type DispatchOption func(*DispatchSettings)

// WithQueue 指定队列名。
func WithQueue(name string) DispatchOption {
	return func(s *DispatchSettings) {
		s.Queue = name
	}
}

// WithDelay 延迟到指定时间后处理。
func WithDelay(t time.Time) DispatchOption {
	return func(s *DispatchSettings) {
		s.Delay = t
	}
}

// WithRetry 设置队列侧最大重试次数（不含首次执行）。
func WithRetry(times int) DispatchOption {
	return func(s *DispatchSettings) {
		s.Retry = times
	}
}
