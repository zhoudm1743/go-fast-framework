package radar

import (
	"context"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// ── 内存 metric reader ──────────────────────────────────────────────

func memMetricSetup(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()
	mr := sdkmetric.NewManualReader()
	RegisterMetricReader("manual", func(ctx context.Context) (sdkmetric.Reader, error) {
		return mr, nil
	})
	return mr
}

func collectMetrics(t *testing.T, mr *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := mr.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	return rm
}

func findMetric(rm metricdata.ResourceMetrics, name string) *metricdata.Metrics {
	for _, sm := range rm.ScopeMetrics {
		for i := range sm.Metrics {
			if sm.Metrics[i].Name == name {
				return &sm.Metrics[i]
			}
		}
	}
	return nil
}

func TestMeter_CounterAndHistogram(t *testing.T) {
	mr := memMetricSetup(t)
	r := newTestRadar(t, map[string]any{"radar.metrics.exporter": "manual"})

	meter := r.Meter("test")
	counter, err := meter.Int64Counter("orders.total", WithDescription("订单总数"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	counter.Add(ctx, 1, String("kind", "vip"))
	counter.Add(ctx, 2, String("kind", "vip"))

	histo, err := meter.Float64Histogram("orders.duration", WithUnit("s"))
	if err != nil {
		t.Fatal(err)
	}
	histo.Record(ctx, 0.5)
	histo.Record(ctx, 1.5)

	updown, err := meter.Int64UpDownCounter("jobs.in_flight")
	if err != nil {
		t.Fatal(err)
	}
	updown.Add(ctx, 3)
	updown.Add(ctx, -1)

	rm := collectMetrics(t, mr)

	// counter 累计 3
	c := findMetric(rm, "orders.total")
	if c == nil {
		t.Fatal("orders.total not found")
	}
	sum, ok := c.Data.(metricdata.Sum[int64])
	if !ok || sum.DataPoints[0].Value != 3 {
		t.Fatalf("counter data = %+v", c.Data)
	}

	// updown 净值 2
	u := findMetric(rm, "jobs.in_flight")
	if u == nil {
		t.Fatal("jobs.in_flight not found")
	}
	if us := u.Data.(metricdata.Sum[int64]); us.DataPoints[0].Value != 2 {
		t.Fatalf("updown = %v", us.DataPoints[0].Value)
	}

	// histogram 两次记录
	h := findMetric(rm, "orders.duration")
	if h == nil {
		t.Fatal("orders.duration not found")
	}
	if hd := h.Data.(metricdata.Histogram[float64]); hd.DataPoints[0].Count != 2 {
		t.Fatalf("histogram count = %v", hd.DataPoints[0].Count)
	}
}

func TestMeter_ObservableGauge(t *testing.T) {
	mr := memMetricSetup(t)
	r := newTestRadar(t, map[string]any{"radar.metrics.exporter": "manual"})

	depth := int64(7)
	err := r.Meter("test").Int64ObservableGauge("queue.depth", func(ctx context.Context) int64 {
		return depth
	})
	if err != nil {
		t.Fatal(err)
	}

	rm := collectMetrics(t, mr)
	g := findMetric(rm, "queue.depth")
	if g == nil {
		t.Fatal("queue.depth not found")
	}
	if gd := g.Data.(metricdata.Gauge[int64]); gd.DataPoints[0].Value != 7 {
		t.Fatalf("gauge = %v", gd.DataPoints[0].Value)
	}
}

func TestMeter_DisabledNoop(t *testing.T) {
	r := newTestRadar(t, map[string]any{})
	c, err := r.Meter("test").Int64Counter("noop.counter")
	if err != nil {
		t.Fatal(err)
	}
	c.Add(context.Background(), 1) // 不应 panic
}

// ── 内存 log exporter ───────────────────────────────────────────────

type memLogExporter struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func (e *memLogExporter) Export(ctx context.Context, records []sdklog.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, r := range records {
		e.records = append(e.records, r.Clone())
	}
	return nil
}

func (e *memLogExporter) Shutdown(context.Context) error   { return nil }
func (e *memLogExporter) ForceFlush(context.Context) error { return nil }

func (e *memLogExporter) snapshot() []sdklog.Record {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]sdklog.Record, len(e.records))
	copy(out, e.records)
	return out
}

func memLogSetup(t *testing.T) *memLogExporter {
	t.Helper()
	exp := &memLogExporter{}
	RegisterLogExporter("memory-log", func(ctx context.Context) (sdklog.Exporter, error) {
		return exp, nil
	})
	return exp
}

func TestLogger_LevelsAndAttrs(t *testing.T) {
	exp := memLogSetup(t)
	r := newTestRadar(t, map[string]any{"radar.logs.exporter": "memory-log"})

	ctx := context.Background()
	lg := r.Logger("test")
	lg.Info(ctx, "order created", String("order.id", "o-1"), Int("items", 2))
	lg.Error(ctx, "charge failed")

	if err := r.ForceFlush(ctx); err != nil {
		t.Fatal(err)
	}
	recs := exp.snapshot()
	if len(recs) != 2 {
		t.Fatalf("records = %d", len(recs))
	}
	if recs[0].Body().AsString() != "order created" {
		t.Fatalf("body = %q", recs[0].Body().AsString())
	}
	if recs[0].Severity().String() != "Info" && recs[0].Severity().String() != "INFO" {
		t.Fatalf("severity = %v", recs[0].Severity())
	}
	var foundOrderID bool
	recs[0].WalkAttributes(func(kv attribute.KeyValue) bool {
		if string(kv.Key) == "order.id" && kv.Value.AsString() == "o-1" {
			foundOrderID = true
		}
		return true
	})
	if !foundOrderID {
		t.Fatal("attribute order.id missing")
	}
	if recs[1].Severity().String() != "Error" && recs[1].Severity().String() != "ERROR" {
		t.Fatalf("severity = %v", recs[1].Severity())
	}
}

func TestLogger_DisabledNoop(t *testing.T) {
	r := newTestRadar(t, map[string]any{})
	r.Logger("test").Info(context.Background(), "noop") // 不应 panic
}
