package radar

import (
	"context"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	otellog "go.opentelemetry.io/otel/log"
	nooplog "go.opentelemetry.io/otel/log/noop"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
)

// radarService 实现 contracts.Radar。
// 未启用的信号使用 otel noop provider，保证零开销且业务无需判空。
type radarService struct {
	tp         *sdktrace.TracerProvider
	mp         *sdkmetric.MeterProvider
	lp         *sdklog.LoggerProvider
	propagator propagation.TextMapPropagator
}

// New 按配置创建 radar 服务。
func New(cfg contracts.Config) (contracts.Radar, error) {
	res := buildResource(cfg)
	s := &radarService{
		propagator: propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{}, propagation.Baggage{},
		),
	}

	// traces
	if name := cfg.GetString("radar.traces.exporter"); name != "" {
		exp, err := buildTraceExporter(cfg, name)
		if err != nil {
			return nil, err
		}
		opts := []sdktrace.TracerProviderOption{
			sdktrace.WithResource(res),
			sdktrace.WithSampler(buildSampler(cfg)),
		}
		if cfg.GetString("radar.traces.processor", "batch") == "simple" {
			opts = append(opts, sdktrace.WithSyncer(exp))
		} else {
			opts = append(opts, sdktrace.WithBatcher(exp))
		}
		s.tp = sdktrace.NewTracerProvider(opts...)
	}

	// metrics
	if name := cfg.GetString("radar.metrics.exporter"); name != "" {
		rd, err := buildMetricReader(cfg, name)
		if err != nil {
			return nil, err
		}
		s.mp = sdkmetric.NewMeterProvider(
			sdkmetric.WithResource(res),
			sdkmetric.WithReader(rd),
		)
	}

	// logs
	if name := cfg.GetString("radar.logs.exporter"); name != "" {
		exp, err := buildLogExporter(cfg, name)
		if err != nil {
			return nil, err
		}
		s.lp = sdklog.NewLoggerProvider(
			sdklog.WithResource(res),
			sdklog.WithProcessor(sdklog.NewBatchProcessor(exp)),
		)
	}

	return s, nil
}

// buildResource 构建服务身份资源（service.name / version / environment）。
func buildResource(cfg contracts.Config) *resource.Resource {
	attrs := []attribute.KeyValue{
		attribute.String("service.name", cfg.GetString("radar.service.name", "gofast-app")),
	}
	if v := cfg.GetString("radar.service.version"); v != "" {
		attrs = append(attrs, attribute.String("service.version", v))
	}
	if v := cfg.GetString("radar.service.environment"); v != "" {
		attrs = append(attrs, attribute.String("deployment.environment", v))
	}
	return resource.NewWithAttributes("", attrs...)
}

// buildSampler 采样器：始终 ParentBased（尊重上游采样决策），根策略按配置。
func buildSampler(cfg contracts.Config) sdktrace.Sampler {
	switch cfg.GetString("radar.traces.sampler", "always_on") {
	case "always_off":
		return sdktrace.ParentBased(sdktrace.NeverSample())
	case "traceidratio":
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.GetFloat64("radar.traces.ratio", 1.0)))
	}
	return sdktrace.ParentBased(sdktrace.AlwaysSample())
}

func (s *radarService) Tracer(name string) contracts.RadarTracer {
	if s.tp == nil {
		return &radarTracer{t: nooptrace.NewTracerProvider().Tracer(name)}
	}
	return &radarTracer{t: s.tp.Tracer(name)}
}

func (s *radarService) Meter(name string) contracts.RadarMeter {
	if s.mp == nil {
		return &radarMeter{m: noopmetric.NewMeterProvider().Meter(name)}
	}
	return &radarMeter{m: s.mp.Meter(name)}
}

func (s *radarService) Logger(name string) contracts.RadarLogger {
	return &radarLogger{l: s.otelLogger(name)}
}

// otelLogger 返回底层 otel logger（zap bridge 等内部接线用）。
func (s *radarService) otelLogger(name string) otellog.Logger {
	if s.lp == nil {
		return nooplog.NewLoggerProvider().Logger(name)
	}
	return s.lp.Logger(name)
}

func (s *radarService) Propagator() contracts.RadarPropagator {
	return &radarPropagator{p: s.propagator}
}

// ForceFlush 立即推送所有已启用信号的缓冲数据。
func (s *radarService) ForceFlush(ctx context.Context) error {
	if s.tp != nil {
		if err := s.tp.ForceFlush(ctx); err != nil {
			return err
		}
	}
	if s.mp != nil {
		if err := s.mp.ForceFlush(ctx); err != nil {
			return err
		}
	}
	if s.lp != nil {
		if err := s.lp.ForceFlush(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Shutdown 关闭所有 provider（app.OnShutdown 调用）。
func (s *radarService) Shutdown(ctx context.Context) error {
	if s.tp != nil {
		_ = s.tp.Shutdown(ctx)
	}
	if s.mp != nil {
		_ = s.mp.Shutdown(ctx)
	}
	if s.lp != nil {
		_ = s.lp.Shutdown(ctx)
	}
	return nil
}

// radarPropagator 包装 otel propagator。
type radarPropagator struct {
	p propagation.TextMapPropagator
}

func (p *radarPropagator) Inject(ctx context.Context, carrier contracts.RadarCarrier) {
	p.p.Inject(ctx, carrierAdapter{carrier})
}

func (p *radarPropagator) Extract(ctx context.Context, carrier contracts.RadarCarrier) context.Context {
	return p.p.Extract(ctx, carrierAdapter{carrier})
}

// carrierAdapter 契约载体 → otel propagation.TextMapCarrier。
type carrierAdapter struct {
	c contracts.RadarCarrier
}

func (a carrierAdapter) Get(key string) string    { return a.c.Get(key) }
func (a carrierAdapter) Set(key, value string)    { a.c.Set(key, value) }
func (a carrierAdapter) Keys() []string           { return a.c.Keys() }

// MapCarrier 把 map[string]string 适配为 RadarCarrier（消息队列等场景）。
type MapCarrier map[string]string

func (m MapCarrier) Get(key string) string { return m[key] }
func (m MapCarrier) Set(key, value string) { m[key] = value }
func (m MapCarrier) Keys() []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
