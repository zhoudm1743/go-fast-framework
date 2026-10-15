package radar

import (
	"context"
	"fmt"
	"math"

	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap/zapcore"
)

// bridgeCore zapcore.Core → OTel log record 桥接。
// 挂载到框架 log 服务后，facades.Log() 的输出自动进入 OTel 日志信号。
type bridgeCore struct {
	logger otellog.Logger
	level  zapcore.LevelEnabler
	fields []zapcore.Field // With() 累积的上下文字段
}

// newBridgeCore 创建桥接核；level 控制最低透传级别。
func newBridgeCore(logger otellog.Logger, level zapcore.LevelEnabler) zapcore.Core {
	return &bridgeCore{logger: logger, level: level}
}

func (c *bridgeCore) Enabled(l zapcore.Level) bool { return c.level.Enabled(l) }

func (c *bridgeCore) With(fields []zapcore.Field) zapcore.Core {
	return &bridgeCore{
		logger: c.logger,
		level:  c.level,
		fields: append(append([]zapcore.Field{}, c.fields...), fields...),
	}
}

func (c *bridgeCore) Check(e zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(e.Level) {
		return ce.AddCore(e, c)
	}
	return ce
}

func (c *bridgeCore) Write(e zapcore.Entry, fields []zapcore.Field) error {
	rec := otellog.Record{}
	rec.SetTimestamp(e.Time)
	rec.SetSeverity(toLogSeverity(e.Level))
	rec.SetSeverityText(e.Level.CapitalString())
	rec.SetBody(attribute.StringValue(e.Message))
	rec.AddAttributes(zapFieldsToAttrs(c.fields)...)
	rec.AddAttributes(zapFieldsToAttrs(fields)...)
	if e.Caller.Defined {
		rec.AddAttributes(attribute.String("caller", e.Caller.String()))
	}
	c.logger.Emit(context.Background(), rec)
	return nil
}

func (c *bridgeCore) Sync() error { return nil }

// toLogSeverity zap 级别 → otel 日志级别。
func toLogSeverity(l zapcore.Level) otellog.Severity {
	switch {
	case l >= zapcore.FatalLevel:
		return otellog.SeverityFatal
	case l >= zapcore.ErrorLevel:
		return otellog.SeverityError
	case l >= zapcore.WarnLevel:
		return otellog.SeverityWarn
	case l >= zapcore.InfoLevel:
		return otellog.SeverityInfo
	default:
		return otellog.SeverityDebug
	}
}

// zapFieldsToAttrs zap 字段 → otel 属性。
func zapFieldsToAttrs(fields []zapcore.Field) []attribute.KeyValue {
	out := make([]attribute.KeyValue, 0, len(fields))
	for _, f := range fields {
		out = append(out, zapFieldToAttr(f))
	}
	return out
}

func zapFieldToAttr(f zapcore.Field) attribute.KeyValue {
	switch f.Type {
	case zapcore.StringType:
		return attribute.String(f.Key, f.String)
	case zapcore.Int64Type, zapcore.Int32Type, zapcore.Int16Type, zapcore.Int8Type,
		zapcore.Uint64Type, zapcore.Uint32Type, zapcore.Uint16Type, zapcore.Uint8Type:
		return attribute.Int64(f.Key, f.Integer)
	case zapcore.Float64Type, zapcore.Float32Type:
		return attribute.Float64(f.Key, math.Float64frombits(uint64(f.Integer)))
	case zapcore.BoolType:
		return attribute.Bool(f.Key, f.Integer == 1)
	case zapcore.ErrorType:
		if err, ok := f.Interface.(error); ok {
			return attribute.String(f.Key, err.Error())
		}
		return attribute.String(f.Key, fmt.Sprint(f.Interface))
	default:
		return attribute.String(f.Key, fmt.Sprint(f.Interface))
	}
}

// traceFieldExtractor log.SetContextFieldExtractor 用的实现：
// 从 std ctx 抽取当前 span 的 trace_id / span_id 作为 zap 字段（所有输出核受益）。
func traceFieldExtractor(ctx context.Context) []zapcore.Field {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return nil
	}
	return []zapcore.Field{
		{Key: "trace_id", Type: zapcore.StringType, String: sc.TraceID().String()},
		{Key: "span_id", Type: zapcore.StringType, String: sc.SpanID().String()},
	}
}
