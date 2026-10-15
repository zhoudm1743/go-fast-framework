package radar

import (
	"fmt"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// ── 属性构造函数 ────────────────────────────────────────────────────

func String(k, v string) contracts.RadarAttr      { return contracts.RadarAttr{Key: k, Value: v} }
func Int(k string, v int) contracts.RadarAttr     { return contracts.RadarAttr{Key: k, Value: v} }
func Int64(k string, v int64) contracts.RadarAttr { return contracts.RadarAttr{Key: k, Value: v} }
func Float64(k string, v float64) contracts.RadarAttr {
	return contracts.RadarAttr{Key: k, Value: v}
}
func Bool(k string, v bool) contracts.RadarAttr { return contracts.RadarAttr{Key: k, Value: v} }

// toOtelAttrs 契约属性 → otel attribute。
func toOtelAttrs(attrs []contracts.RadarAttr) []attribute.KeyValue {
	out := make([]attribute.KeyValue, 0, len(attrs))
	for _, a := range attrs {
		switch v := a.Value.(type) {
		case string:
			out = append(out, attribute.String(a.Key, v))
		case int:
			out = append(out, attribute.Int(a.Key, v))
		case int64:
			out = append(out, attribute.Int64(a.Key, v))
		case float64:
			out = append(out, attribute.Float64(a.Key, v))
		case bool:
			out = append(out, attribute.Bool(a.Key, v))
		case []string:
			out = append(out, attribute.StringSlice(a.Key, v))
		case []int:
			out = append(out, attribute.IntSlice(a.Key, v))
		case []int64:
			out = append(out, attribute.Int64Slice(a.Key, v))
		case []float64:
			out = append(out, attribute.Float64Slice(a.Key, v))
		case []bool:
			out = append(out, attribute.BoolSlice(a.Key, v))
		default:
			out = append(out, attribute.String(a.Key, fmt.Sprint(v)))
		}
	}
	return out
}

// ── Span 选项 ───────────────────────────────────────────────────────

// WithSpanKind 声明 Span 角色（边界操作：Server/Client/Producer/Consumer）。
func WithSpanKind(kind contracts.RadarSpanKind) contracts.RadarSpanOption {
	return func(c *contracts.RadarSpanConfig) { c.Kind = kind }
}

// WithAttributes 创建时附加属性。
func WithAttributes(attrs ...contracts.RadarAttr) contracts.RadarSpanOption {
	return func(c *contracts.RadarSpanConfig) { c.Attrs = append(c.Attrs, attrs...) }
}

// WithTimestamp 指定 Span 起始时间（默认当前时间）。
func WithTimestamp(t time.Time) contracts.RadarSpanOption {
	return func(c *contracts.RadarSpanConfig) { c.Time = t }
}

// ── 仪器选项 ────────────────────────────────────────────────────────

// WithDescription 仪器描述。
func WithDescription(d string) contracts.RadarInstrumentOption {
	return func(c *contracts.RadarInstrumentConfig) { c.Description = d }
}

// WithUnit 仪器单位（如 "s"、"By"）。
func WithUnit(u string) contracts.RadarInstrumentOption {
	return func(c *contracts.RadarInstrumentConfig) { c.Unit = u }
}

// ── 映射 ────────────────────────────────────────────────────────────

func toSpanKind(k contracts.RadarSpanKind) trace.SpanKind {
	switch k {
	case contracts.RadarSpanKindServer:
		return trace.SpanKindServer
	case contracts.RadarSpanKindClient:
		return trace.SpanKindClient
	case contracts.RadarSpanKindProducer:
		return trace.SpanKindProducer
	case contracts.RadarSpanKindConsumer:
		return trace.SpanKindConsumer
	}
	return trace.SpanKindInternal
}

func toStatusCode(c contracts.RadarStatusCode) codes.Code {
	switch c {
	case contracts.RadarStatusOK:
		return codes.Ok
	case contracts.RadarStatusError:
		return codes.Error
	}
	return codes.Unset
}
