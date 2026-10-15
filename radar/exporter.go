package radar

import (
	"context"
	"fmt"
	"sync"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// ── 自定义导出器注册表（代码级，不进 YAML）─────────────────────────

type (
	// TraceExporterFactory 自定义 trace 导出器工厂。
	TraceExporterFactory func(ctx context.Context) (sdktrace.SpanExporter, error)
	// MetricReaderFactory 自定义 metric 读取器工厂。
	MetricReaderFactory func(ctx context.Context) (sdkmetric.Reader, error)
	// LogExporterFactory 自定义 log 导出器工厂。
	LogExporterFactory func(ctx context.Context) (sdklog.Exporter, error)
)

var (
	registryMu        sync.RWMutex
	traceExporters    = map[string]TraceExporterFactory{}
	metricReaders     = map[string]MetricReaderFactory{}
	logExporters      = map[string]LogExporterFactory{}
)

// RegisterTraceExporter 注册自定义 trace 导出器（配置 traces.exporter 引用该名字）。
func RegisterTraceExporter(name string, f TraceExporterFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	traceExporters[name] = f
}

// RegisterMetricReader 注册自定义 metric 读取器。
func RegisterMetricReader(name string, f MetricReaderFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	metricReaders[name] = f
}

// RegisterLogExporter 注册自定义 log 导出器。
func RegisterLogExporter(name string, f LogExporterFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	logExporters[name] = f
}

// 注销函数仅测试使用。
func unregisterTraceExporter(name string) {
	registryMu.Lock()
	defer registryMu.Unlock()
	delete(traceExporters, name)
}

func findTraceExporter(name string) (TraceExporterFactory, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	f, ok := traceExporters[name]
	return f, ok
}

func findMetricReader(name string) (MetricReaderFactory, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	f, ok := metricReaders[name]
	return f, ok
}

func findLogExporter(name string) (LogExporterFactory, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	f, ok := logExporters[name]
	return f, ok
}

// ── 内置 driver 构造 ────────────────────────────────────────────────

// exporterCfg 读取 radar.exporters.<name> 配置节。
func exporterCfg(cfg contracts.Config, name string) (endpoint string, insecure bool, headers map[string]string, driver string) {
	prefix := "radar.exporters." + name
	driver = cfg.GetString(prefix + ".driver")
	endpoint = cfg.GetString(prefix+".endpoint", "localhost:4318")
	insecure = cfg.GetBool(prefix+".insecure", true)
	headers = map[string]string{}
	for k, v := range cfg.GetStringMap(prefix + ".headers") {
		if s, ok := v.(string); ok {
			headers[k] = s
		}
	}
	return
}

func buildTraceExporter(cfg contracts.Config, name string) (sdktrace.SpanExporter, error) {
	if f, ok := findTraceExporter(name); ok {
		return f(context.Background())
	}
	endpoint, insecure, headers, driver := exporterCfg(cfg, name)
	switch driver {
	case "otlp":
		opts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(endpoint)}
		if insecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		if len(headers) > 0 {
			opts = append(opts, otlptracehttp.WithHeaders(headers))
		}
		return otlptracehttp.New(context.Background(), opts...)
	case "console":
		return stdouttrace.New(stdouttrace.WithPrettyPrint())
	}
	return nil, fmt.Errorf("radar: 未知 trace 导出器 %q（driver %q 未支持，且未注册 custom 工厂）", name, driver)
}

func buildMetricReader(cfg contracts.Config, name string) (sdkmetric.Reader, error) {
	if f, ok := findMetricReader(name); ok {
		return f(context.Background())
	}
	endpoint, insecure, headers, driver := exporterCfg(cfg, name)
	interval := getDuration(cfg, "radar.metrics.interval", 60*1000*1000*1000)
	switch driver {
	case "otlp":
		opts := []otlpmetrichttp.Option{otlpmetrichttp.WithEndpoint(endpoint)}
		if insecure {
			opts = append(opts, otlpmetrichttp.WithInsecure())
		}
		if len(headers) > 0 {
			opts = append(opts, otlpmetrichttp.WithHeaders(headers))
		}
		exp, err := otlpmetrichttp.New(context.Background(), opts...)
		if err != nil {
			return nil, err
		}
		return sdkmetric.NewPeriodicReader(exp, sdkmetric.WithInterval(interval)), nil
	case "console":
		exp, err := stdoutmetric.New()
		if err != nil {
			return nil, err
		}
		return sdkmetric.NewPeriodicReader(exp, sdkmetric.WithInterval(interval)), nil
	}
	return nil, fmt.Errorf("radar: 未知 metric 导出器 %q", name)
}

func buildLogExporter(cfg contracts.Config, name string) (sdklog.Exporter, error) {
	if f, ok := findLogExporter(name); ok {
		return f(context.Background())
	}
	endpoint, insecure, headers, driver := exporterCfg(cfg, name)
	switch driver {
	case "otlp":
		opts := []otlploghttp.Option{otlploghttp.WithEndpoint(endpoint)}
		if insecure {
			opts = append(opts, otlploghttp.WithInsecure())
		}
		if len(headers) > 0 {
			opts = append(opts, otlploghttp.WithHeaders(headers))
		}
		return otlploghttp.New(context.Background(), opts...)
	case "console":
		return stdoutlog.New()
	}
	return nil, fmt.Errorf("radar: 未知 log 导出器 %q", name)
}
