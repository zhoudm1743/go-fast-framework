package radar

import (
	"context"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"go.opentelemetry.io/otel/trace"
)

// radarTracer 包装 otel trace.Tracer。
type radarTracer struct {
	t trace.Tracer
}

func (t *radarTracer) Start(ctx context.Context, name string, opts ...contracts.RadarSpanOption) (context.Context, contracts.RadarSpan) {
	cfg := &contracts.RadarSpanConfig{}
	for _, o := range opts {
		o(cfg)
	}
	otelOpts := []trace.SpanStartOption{
		trace.WithSpanKind(toSpanKind(cfg.Kind)),
	}
	if len(cfg.Attrs) > 0 {
		otelOpts = append(otelOpts, trace.WithAttributes(toOtelAttrs(cfg.Attrs)...))
	}
	if !cfg.Time.IsZero() {
		otelOpts = append(otelOpts, trace.WithTimestamp(cfg.Time))
	}
	c, s := t.t.Start(ctx, name, otelOpts...)
	return c, &radarSpan{s: s}
}

// radarSpan 包装 otel trace.Span。
type radarSpan struct {
	s trace.Span
}

func (s *radarSpan) End() { s.s.End() }

func (s *radarSpan) SetAttributes(attrs ...contracts.RadarAttr) {
	s.s.SetAttributes(toOtelAttrs(attrs)...)
}

func (s *radarSpan) AddEvent(name string, attrs ...contracts.RadarAttr) {
	s.s.AddEvent(name, trace.WithAttributes(toOtelAttrs(attrs)...))
}

func (s *radarSpan) RecordError(err error) { s.s.RecordError(err) }

func (s *radarSpan) SetStatus(code contracts.RadarStatusCode, msg string) {
	s.s.SetStatus(toStatusCode(code), msg)
}

func (s *radarSpan) TraceID() string {
	sc := s.s.SpanContext()
	if !sc.HasTraceID() {
		return ""
	}
	return sc.TraceID().String()
}

func (s *radarSpan) SpanID() string {
	sc := s.s.SpanContext()
	if !sc.HasSpanID() {
		return ""
	}
	return sc.SpanID().String()
}

// SpanFromCtx 从框架 http Context 取当前请求 Span（埋点中间件写入）。
// 不存在时返回 nil，调用方需判空。
func SpanFromCtx(ctx contracts.Context) contracts.RadarSpan {
	if ctx == nil {
		return nil
	}
	if v := ctx.Value(string(contracts.CtxKeyRadarSpan)); v != nil {
		if span, ok := v.(contracts.RadarSpan); ok {
			return span
		}
	}
	return nil
}

// StdCtx 从框架 http Context 取携带当前 Span 的标准 context.Context（埋点中间件写入）。
// 不存在时返回 context.Background()。
func StdCtx(ctx contracts.Context) context.Context {
	if ctx != nil {
		if v := ctx.Value(string(contracts.CtxKeyRadarStdCtx)); v != nil {
			if c, ok := v.(context.Context); ok {
				return c
			}
		}
	}
	return context.Background()
}
