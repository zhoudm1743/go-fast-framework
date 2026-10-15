package radar

import (
	"context"
	"testing"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/facades"
	"github.com/zhoudm1743/go-fast-framework/foundation"
	"github.com/zhoudm1743/go-fast-framework/log"
	"github.com/zhoudm1743/go-fast-framework/queue"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type integrationJob struct{ done chan struct{} }

func (j integrationJob) Signature() string { return "test:integration" }
func (j integrationJob) Handle(...any) error {
	close(j.done)
	return nil
}

// TestRadarIntegration 端到端：Boot 接线 → facades.Radar() → queue span + log 桥接。
func TestRadarIntegration(t *testing.T) {
	inmem := tracetest.NewInMemoryExporter()
	RegisterTraceExporter("int-trace", func(ctx context.Context) (sdktrace.SpanExporter, error) {
		return inmem, nil
	})
	logExp := &memLogExporter{}
	RegisterLogExporter("int-log", func(ctx context.Context) (sdklog.Exporter, error) {
		return logExp, nil
	})

	cfg := newRadarTestConfig(map[string]any{
		"radar.service.name":      "int-app",
		"radar.traces.exporter":   "int-trace",
		"radar.traces.processor":  "simple",
		"radar.logs.exporter":     "int-log",
		"log.mode":                "console",
		"log.level":               "debug",
	})

	logSvc, err := log.NewLogger(cfg)
	if err != nil {
		t.Fatal(err)
	}

	app := foundation.NewApplication(".")
	app.Instance("config", cfg)
	app.Instance("log", logSvc)
	app.Instance("queue", queue.New(logSvc, queue.WithWorkers(1)))
	app.SetProviders([]foundation.ServiceProvider{&ServiceProvider{}})
	app.Boot()
	defer app.Shutdown()

	// 1. 门面可用
	r := facades.Radar()
	if r == nil {
		t.Fatal("facades.Radar() nil")
	}

	// 2. queue 埋点已挂载：执行任务产生 consumer span
	j := integrationJob{done: make(chan struct{})}
	if err := facades.Queue().Job(j, nil).Dispatch(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-j.done:
	case <-time.After(3 * time.Second):
		t.Fatal("job not executed")
	}
	time.Sleep(50 * time.Millisecond)
	var queueSpanFound bool
	for _, s := range inmem.GetSpans() {
		if s.Name == "queue.exec test:integration" {
			queueSpanFound = true
		}
	}
	if !queueSpanFound {
		t.Fatalf("queue span missing: %v", inmem.GetSpans())
	}

	// 3. log 桥接已挂载：框架日志进入 OTel logs 信号
	facades.Log().Info("integration log line")
	_ = r.ForceFlush(context.Background())
	found := false
	for _, rec := range logExp.snapshot() {
		if rec.Body().AsString() == "integration log line" {
			found = true
		}
	}
	if !found {
		t.Fatal("log bridge record missing")
	}
}

var _ contracts.Radar = (*radarService)(nil)
