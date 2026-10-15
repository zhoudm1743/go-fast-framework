package middleware

import (
	"context"
	"testing"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/facades"
	"github.com/zhoudm1743/go-fast-framework/foundation"
	"github.com/zhoudm1743/go-fast-framework/radar"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// radarMwSetup 装配带内存 trace/metric 导出的 radar 服务并注入全局 app。
func radarMwSetup(t *testing.T) (*tracetest.InMemoryExporter, *sdkmetric.ManualReader) {
	t.Helper()
	inmem := tracetest.NewInMemoryExporter()
	radar.RegisterTraceExporter("mw-trace", func(ctx context.Context) (sdktrace.SpanExporter, error) {
		return inmem, nil
	})
	mr := sdkmetric.NewManualReader()
	radar.RegisterMetricReader("mw-metric", func(ctx context.Context) (sdkmetric.Reader, error) {
		return mr, nil
	})

	svc, err := radar.New(&mwTestConfig{m: map[string]any{
		"radar.traces.exporter":  "mw-trace",
		"radar.metrics.exporter": "mw-metric",
		"radar.traces.processor": "simple",
	}})
	if err != nil {
		t.Fatal(err)
	}
	app := foundation.NewApplication(".")
	app.Instance("radar", svc)
	facades.SetApp(app)
	return inmem, mr
}

func TestRadarMiddleware_ServerSpanAndPropagation(t *testing.T) {
	inmem, _ := radarMwSetup(t)

	// 上游传来的 traceparent
	upstreamTrace := "0af7651916cd43dd8448eb211c80319c"
	ctx := &mwCtx{
		method:   "GET",
		fullPath: "/users/:id",
		headers: map[string]string{
			"Traceparent": "00-" + upstreamTrace + "-b7ad6b7169203331-01",
		},
		values: map[string]any{},
		resp:   &mwResponse{},
	}

	if err := Radar()(ctx); err != nil {
		t.Fatal(err)
	}
	if !ctx.nextCalled {
		t.Fatal("next must be called")
	}

	spans := inmem.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d", len(spans))
	}
	s := spans[0]
	if s.Name != "GET /users/:id" {
		t.Fatalf("span name = %q", s.Name)
	}
	if s.SpanKind != 2 { // server
		t.Fatalf("kind = %v", s.SpanKind)
	}
	// 传播：trace id 与上游一致
	if s.SpanContext.TraceID().String() != upstreamTrace {
		t.Fatalf("trace id = %s, want upstream %s", s.SpanContext.TraceID(), upstreamTrace)
	}
	// ctx 注入了 span 与 std ctx
	if radar.SpanFromCtx(ctx) == nil {
		t.Fatal("CtxKeyRadarSpan must be set")
	}
	if radar.StdCtx(ctx) == nil {
		t.Fatal("CtxKeyRadarStdCtx must be set")
	}
}

func TestRadarMiddleware_RecordsDurationMetric(t *testing.T) {
	_, mr := radarMwSetup(t)

	ctx := &mwCtx{method: "POST", fullPath: "/orders", headers: map[string]string{}, values: map[string]any{}, resp: &mwResponse{}}
	if err := Radar()(ctx); err != nil {
		t.Fatal(err)
	}

	var rm metricdata.ResourceMetrics
	if err := mr.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "http.server.request.duration" {
				found = true
				if h := m.Data.(metricdata.Histogram[float64]); h.DataPoints[0].Count != 1 {
					t.Fatalf("duration count = %v", h.DataPoints[0].Count)
				}
			}
		}
	}
	if !found {
		t.Fatal("http.server.request.duration not recorded")
	}
}

func TestRadarMiddleware_NoUpstreamHeader(t *testing.T) {
	inmem, _ := radarMwSetup(t)
	ctx := &mwCtx{method: "GET", fullPath: "/health", headers: map[string]string{}, values: map[string]any{}, resp: &mwResponse{}}
	if err := Radar()(ctx); err != nil {
		t.Fatal(err)
	}
	spans := inmem.GetSpans()
	if len(spans) != 1 || !spans[0].SpanContext.IsValid() {
		t.Fatalf("spans = %+v", spans)
	}
	_ = time.Now
}

var _ = contracts.RadarSpanKindServer
