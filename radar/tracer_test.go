package radar

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// memTraceSetup 注册内存 trace exporter 并返回 recorder。
func memTraceSetup(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	inmem := tracetest.NewInMemoryExporter()
	RegisterTraceExporter("memory", func(ctx context.Context) (sdktrace.SpanExporter, error) {
		return inmem, nil
	})
	t.Cleanup(func() { unregisterTraceExporter("memory") })
	return inmem
}

func newTestRadar(t *testing.T, extra map[string]any) contracts.Radar {
	t.Helper()
	m := map[string]any{
		"radar.service.name": "test-app",
	}
	for k, v := range extra {
		m[k] = v
	}
	cfg := newRadarTestConfig(m)
	r, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.(*radarService).Shutdown(context.Background()) })
	return r
}

func TestTracer_SpanRecording(t *testing.T) {
	inmem := memTraceSetup(t)
	r := newTestRadar(t, map[string]any{"radar.traces.exporter": "memory"})

	ctx, span := r.Tracer("test").Start(context.Background(), "root-op",
		WithAttributes(String("key1", "v1"), Int("count", 3)))
	span.AddEvent("checkpoint", String("stage", "mid"))
	_, child := r.Tracer("test").Start(ctx, "child-op")
	child.End()
	span.SetStatus(contracts.RadarStatusOK, "done")
	span.End()

	if err := r.ForceFlush(context.Background()); err != nil {
		t.Fatal(err)
	}
	spans := inmem.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}
	var root, childSpan *tracetest.SpanStub
	for i := range spans {
		switch spans[i].Name {
		case "root-op":
			root = &spans[i]
		case "child-op":
			childSpan = &spans[i]
		}
	}
	if root == nil || childSpan == nil {
		t.Fatalf("spans not found: %v", spans)
	}
	// 父子关系：同一 trace，child 的 parent 是 root
	if childSpan.SpanContext.TraceID() != root.SpanContext.TraceID() {
		t.Fatal("child must share root trace id")
	}
	if childSpan.Parent.SpanID() != root.SpanContext.SpanID() {
		t.Fatal("child parent must be root span")
	}
	// 属性与事件
	foundAttr := false
	for _, a := range root.Attributes {
		if a.Key == "key1" && a.Value.AsString() == "v1" {
			foundAttr = true
		}
	}
	if !foundAttr {
		t.Fatalf("attribute key1 missing: %v", root.Attributes)
	}
	if len(root.Events) != 1 || root.Events[0].Name != "checkpoint" {
		t.Fatalf("events = %v", root.Events)
	}
}

func TestTracer_SpanErrorRecording(t *testing.T) {
	inmem := memTraceSetup(t)
	r := newTestRadar(t, map[string]any{"radar.traces.exporter": "memory"})

	_, span := r.Tracer("test").Start(context.Background(), "fail-op")
	span.RecordError(errors.New("boom"))
	span.SetStatus(contracts.RadarStatusError, "failed")
	span.End()
	_ = r.ForceFlush(context.Background())

	spans := inmem.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d", len(spans))
	}
	if spans[0].Status.Code.String() != "Error" {
		t.Fatalf("status = %v", spans[0].Status)
	}
}

func TestTracer_SpanIDs(t *testing.T) {
	_ = memTraceSetup(t)
	r := newTestRadar(t, map[string]any{"radar.traces.exporter": "memory"})

	_, span := r.Tracer("test").Start(context.Background(), "op")
	if span.TraceID() == "" || span.SpanID() == "" {
		t.Fatal("span must expose trace/span ids")
	}
	span.End()
}

func TestTracer_DisabledNoop(t *testing.T) {
	r := newTestRadar(t, map[string]any{}) // 未配置任何 exporter

	_, span := r.Tracer("test").Start(context.Background(), "op")
	span.SetAttributes(String("k", "v"))
	span.RecordError(errors.New("x"))
	span.End()

	if err := r.ForceFlush(context.Background()); err != nil {
		t.Fatal(err)
	}
	// no-op span 的 ID 为空（零开销路径）
	if span.TraceID() != "" {
		t.Fatalf("noop span trace id must be empty, got %q", span.TraceID())
	}
}

func TestTracer_SpanKindAndTime(t *testing.T) {
	inmem := memTraceSetup(t)
	r := newTestRadar(t, map[string]any{"radar.traces.exporter": "memory"})

	start := time.Now().Add(-time.Second)
	_, span := r.Tracer("test").Start(context.Background(), "server-op",
		WithSpanKind(contracts.RadarSpanKindServer), WithTimestamp(start))
	span.End()
	_ = r.ForceFlush(context.Background())

	spans := inmem.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d", len(spans))
	}
	if spans[0].SpanKind != trace.SpanKindServer {
		t.Fatalf("kind = %v", spans[0].SpanKind)
	}
	if !spans[0].StartTime.Equal(start) {
		t.Fatalf("start time = %v, want %v", spans[0].StartTime, start)
	}
}
