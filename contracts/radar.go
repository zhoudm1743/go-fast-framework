package contracts

import (
	"context"
	"time"
)

// ── 可观测性契约（radar）────────────────────────────────────────────

// Radar 可观测性服务契约（facades.Radar() 返回此接口）。
// 提供 OpenTelemetry 三信号：追踪（traces）、指标（metrics）、日志（logs）。
// 全部信号未启用时返回 no-op 实现，业务代码无需判空。
type Radar interface {
	// Tracer 获取追踪器；name 为埋点作用域名（惯例：包路径或模块名）。
	Tracer(name string) RadarTracer
	// Meter 获取指标器。
	Meter(name string) RadarMeter
	// Logger 获取 OTel 日志记录器（绕过 log 门面直发 OTel 日志）。
	Logger(name string) RadarLogger
	// Propagator 追踪上下文传播器（W3C tracecontext + baggage）。
	Propagator() RadarPropagator
	// ForceFlush 立即推送缓冲数据（不停止 provider）。
	ForceFlush(ctx context.Context) error
}

// RadarTracer 追踪器。
type RadarTracer interface {
	// Start 开启 Span；ctx 中已有活跃 Span 时自动成为其子 Span。
	Start(ctx context.Context, name string, opts ...RadarSpanOption) (context.Context, RadarSpan)
}

// RadarSpan 一个定时操作（如 HTTP 请求、DB 查询、函数调用）。
type RadarSpan interface {
	// End 结束 Span（惯例 defer span.End()）。
	End()
	// SetAttributes 附加键值属性。
	SetAttributes(attrs ...RadarAttr)
	// AddEvent 记录 Span 内的时间点事件（如缓存未命中）。
	AddEvent(name string, attrs ...RadarAttr)
	// RecordError 将错误作为事件附加（不改变 Span 状态，需配合 SetStatus）。
	RecordError(err error)
	// SetStatus 设置 Span 状态（RadarStatusError 标记失败）。
	SetStatus(code RadarStatusCode, msg string)
	// TraceID / SpanID 当前标识（可回传到错误响应）。
	TraceID() string
	SpanID() string
}

// RadarStatusCode Span 状态码。
type RadarStatusCode int

const (
	RadarStatusUnset RadarStatusCode = iota
	RadarStatusOK
	RadarStatusError
)

// RadarSpanKind Span 角色（边界操作声明用）。
type RadarSpanKind int

const (
	RadarSpanKindInternal RadarSpanKind = iota
	RadarSpanKindServer
	RadarSpanKindClient
	RadarSpanKindProducer
	RadarSpanKindConsumer
)

// RadarAttr Span/指标/日志的键值属性。
// Value 支持 string / int / int64 / float64 / bool 及对应切片。
type RadarAttr struct {
	Key   string
	Value any
}

// RadarSpanConfig Span 创建选项集合。
type RadarSpanConfig struct {
	Kind   RadarSpanKind
	Attrs  []RadarAttr
	Time   time.Time
}

// RadarSpanOption Span 创建选项函数。
type RadarSpanOption func(*RadarSpanConfig)

// RadarMeter 指标器。仪器并发安全，应创建一次重复使用（如服务构造时创建）。
type RadarMeter interface {
	// Int64Counter 单调递增计数器（如已处理订单数）。
	Int64Counter(name string, opts ...RadarInstrumentOption) (RadarInt64Counter, error)
	// Int64UpDownCounter 可增可减计数器（如在处理任务数）。
	Int64UpDownCounter(name string, opts ...RadarInstrumentOption) (RadarInt64Counter, error)
	// Float64Histogram 分布直方图（如耗时、载荷大小）。
	Float64Histogram(name string, opts ...RadarInstrumentOption) (RadarFloat64Histogram, error)
	// Int64ObservableGauge 可观测计量表：注册回调，每个采集周期调用（如队列深度）。
	Int64ObservableGauge(name string, callback func(ctx context.Context) int64, opts ...RadarInstrumentOption) error
}

// RadarInt64Counter int64 计数器（Counter / UpDownCounter 共用）。
type RadarInt64Counter interface {
	Add(ctx context.Context, value int64, attrs ...RadarAttr)
}

// RadarFloat64Histogram float64 直方图。
type RadarFloat64Histogram interface {
	Record(ctx context.Context, value float64, attrs ...RadarAttr)
}

// RadarInstrumentConfig 仪器选项集合。
type RadarInstrumentConfig struct {
	Description string
	Unit        string
}

// RadarInstrumentOption 仪器选项函数。
type RadarInstrumentOption func(*RadarInstrumentConfig)

// RadarLogger OTel 日志记录器（带 trace 关联）。
type RadarLogger interface {
	Debug(ctx context.Context, msg string, attrs ...RadarAttr)
	Info(ctx context.Context, msg string, attrs ...RadarAttr)
	Warn(ctx context.Context, msg string, attrs ...RadarAttr)
	Error(ctx context.Context, msg string, attrs ...RadarAttr)
}

// RadarPropagator 跨进程追踪上下文传播。
type RadarPropagator interface {
	// Inject 把活跃追踪上下文注入载体（如 HTTP 头 / 消息头）。
	Inject(ctx context.Context, carrier RadarCarrier)
	// Extract 从载体提取追踪上下文，继续同一条 trace。
	Extract(ctx context.Context, carrier RadarCarrier) context.Context
}

// RadarCarrier 文本载体（map / HTTP 头适配）。
type RadarCarrier interface {
	Get(key string) string
	Set(key, value string)
	Keys() []string
}
