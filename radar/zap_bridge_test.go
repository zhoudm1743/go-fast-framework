package radar

import (
	"context"
	"testing"

	"github.com/zhoudm1743/go-fast-framework/log"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestZapBridge_LevelAndFieldMapping(t *testing.T) {
	exp := memLogSetup(t)
	r := newTestRadar(t, map[string]any{"radar.logs.exporter": "memory-log"})

	core := newBridgeCore(r.(*radarService).otelLogger("app"), zapcore.DebugLevel)
	zapLog := zap.New(core).Sugar()
	zapLog.Infow("order created", "order_id", "o-1", "items", 2)
	zapLog.Errorw("charge failed", "error", "card declined")

	if err := r.ForceFlush(context.Background()); err != nil {
		t.Fatal(err)
	}
	recs := exp.snapshot()
	if len(recs) != 2 {
		t.Fatalf("records = %d", len(recs))
	}
	if recs[0].Body().AsString() != "order created" {
		t.Fatalf("body = %q", recs[0].Body().AsString())
	}
	var foundOrderID, foundItems bool
	recs[0].WalkAttributes(func(kv attribute.KeyValue) bool {
		switch string(kv.Key) {
		case "order_id":
			foundOrderID = kv.Value.AsString() == "o-1"
		case "items":
			foundItems = kv.Value.AsInt64() == 2
		}
		return true
	})
	if !foundOrderID || !foundItems {
		t.Fatal("zap fields not mapped to otel attributes")
	}
	if recs[1].Severity().String() != "Error" && recs[1].Severity().String() != "ERROR" {
		t.Fatalf("severity = %v", recs[1].Severity())
	}
}

func TestZapBridge_TraceCorrelationViaExtractor(t *testing.T) {
	_ = memTraceSetup(t)
	exp := memLogSetup(t)
	r := newTestRadar(t, map[string]any{
		"radar.traces.exporter": "memory",
		"radar.logs.exporter":   "memory-log",
	})

	// 模拟 radar Boot 的接线：注册 ctx 字段抽取器
	log.SetContextFieldExtractor(traceFieldExtractor)
	t.Cleanup(func() { log.SetContextFieldExtractor(nil) })

	// 开 span 的 std ctx
	stdCtx, span := r.Tracer("test").Start(context.Background(), "req-op")
	defer span.End()

	// 业务侧：facades.Log().WithContext(ctx) 路径（此处直接构造 logEntry 路径）
	core := newBridgeCore(r.(*radarService).otelLogger("app"), zapcore.DebugLevel)
	zapLog := zap.New(core).Sugar()
	// WithContext 由 log 包处理，这里直接验证 extractor + bridge 的组合：
	// 用 extractor 产出的字段写入
	fields := traceFieldExtractor(stdCtx)
	args := make([]any, len(fields))
	for i, f := range fields {
		args[i] = f
	}
	zapLog.With(args...).Info("in request")

	_ = r.ForceFlush(context.Background())
	recs := exp.snapshot()
	if len(recs) != 1 {
		t.Fatalf("records = %d", len(recs))
	}
	var traceID string
	recs[0].WalkAttributes(func(kv attribute.KeyValue) bool {
		if string(kv.Key) == "trace_id" {
			traceID = kv.Value.AsString()
		}
		return true
	})
	if traceID == "" || traceID != span.TraceID() {
		t.Fatalf("trace_id = %q, want %q", traceID, span.TraceID())
	}
}

func TestZapBridge_LevelFilter(t *testing.T) {
	exp := memLogSetup(t)
	r := newTestRadar(t, map[string]any{"radar.logs.exporter": "memory-log"})

	core := newBridgeCore(r.(*radarService).otelLogger("app"), zapcore.WarnLevel)
	zapLog := zap.New(core).Sugar()
	zapLog.Info("filtered out")
	zapLog.Warn("kept")

	_ = r.ForceFlush(context.Background())
	if n := len(exp.snapshot()); n != 1 {
		t.Fatalf("records = %d, want 1 (info filtered)", n)
	}
}
