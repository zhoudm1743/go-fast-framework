package radar

import (
	"context"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/foundation"
	"github.com/zhoudm1743/go-fast-framework/log"
	"go.uber.org/zap/zapcore"
)

// ServiceProvider Radar 服务提供者。
type ServiceProvider struct{}

func (sp *ServiceProvider) Register(app foundation.Application) {
	app.Singleton("radar", func(app foundation.Application) (any, error) {
		return New(app.Config())
	})
}

func (sp *ServiceProvider) Boot(app foundation.Application) error {
	cfg := app.Config()

	// 优雅关闭：flush 并关闭所有 provider（上限 15s）
	app.OnShutdown(func() {
		if r, err := app.Make("radar"); err == nil {
			if svc, ok := r.(*radarService); ok {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				_ = svc.Shutdown(ctx)
			}
		}
	})

	// queue 埋点：鸭子类型挂载（queue.manager.SetTracer）
	if cfg.GetBool("radar.instrumentation.queue", true) && app.Bound("queue") {
		if q, err := app.Make("queue"); err == nil {
			if st, ok := q.(interface{ SetTracer(contracts.RadarTracer) }); ok {
				if r, err := app.Make("radar"); err == nil {
					st.SetTracer(r.(contracts.Radar).Tracer("gofast/queue"))
				}
			}
		}
	}

	// log 桥接：logs 信号启用且允许桥接时，把框架日志接入 OTel
	logsEnabled := cfg.GetString("radar.logs.exporter") != ""
	if logsEnabled && cfg.GetBool("radar.instrumentation.log_bridge", true) {
		// trace_id/span_id 关联（console/file/otel 所有输出核受益）
		log.SetContextFieldExtractor(traceFieldExtractor)
		if l, err := app.Make("log"); err == nil {
			if ac, ok := l.(interface{ AddCore(zapcore.Core) }); ok {
				if r, err := app.Make("radar"); err == nil {
					if svc, ok := r.(*radarService); ok {
						ac.AddCore(newBridgeCore(svc.otelLogger("gofast/log"), bridgeLevel(cfg)))
					}
				}
			}
		}
	}
	return nil
}

// bridgeLevel 桥接核的最低级别（沿用 log.level，默认 info）。
func bridgeLevel(cfg contracts.Config) zapcore.LevelEnabler {
	var lv zapcore.Level
	if err := lv.UnmarshalText([]byte(cfg.GetString("log.level", "info"))); err != nil {
		lv = zapcore.InfoLevel
	}
	return lv
}

// ConfigDefaults 声明 radar 的默认配置。
func (sp *ServiceProvider) ConfigDefaults() map[string]any {
	return map[string]any{
		"radar.traces.sampler":              "always_on",
		"radar.traces.processor":            "batch",
		"radar.metrics.interval":            "60s",
		"radar.instrumentation.http_server": true,
		"radar.instrumentation.queue":       true,
		"radar.instrumentation.log_bridge":  true,
	}
}
