package radar

import (
	"context"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
)

// radarLogger 包装 otel log.Logger。
type radarLogger struct {
	l otellog.Logger
}

func (l *radarLogger) Debug(ctx context.Context, msg string, attrs ...contracts.RadarAttr) {
	l.emit(ctx, otellog.SeverityDebug, msg, attrs)
}

func (l *radarLogger) Info(ctx context.Context, msg string, attrs ...contracts.RadarAttr) {
	l.emit(ctx, otellog.SeverityInfo, msg, attrs)
}

func (l *radarLogger) Warn(ctx context.Context, msg string, attrs ...contracts.RadarAttr) {
	l.emit(ctx, otellog.SeverityWarn, msg, attrs)
}

func (l *radarLogger) Error(ctx context.Context, msg string, attrs ...contracts.RadarAttr) {
	l.emit(ctx, otellog.SeverityError, msg, attrs)
}

func (l *radarLogger) emit(ctx context.Context, sev otellog.Severity, msg string, attrs []contracts.RadarAttr) {
	rec := otellog.Record{}
	rec.SetTimestamp(time.Now())
	rec.SetSeverity(sev)
	rec.SetSeverityText(sev.String())
	rec.SetBody(attribute.StringValue(msg))
	rec.AddAttributes(toOtelAttrs(attrs)...)
	l.l.Emit(ctx, rec)
}
