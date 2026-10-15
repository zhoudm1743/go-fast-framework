package radar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestTransport_InjectsTraceparentAndClientSpan(t *testing.T) {
	inmem := tracetest.NewInMemoryExporter()
	RegisterTraceExporter("transport-trace", func(ctx context.Context) (sdktrace.SpanExporter, error) {
		return inmem, nil
	})
	t.Cleanup(func() { unregisterTraceExporter("transport-trace") })

	r := newTestRadar(t, map[string]any{
		"radar.traces.exporter":  "transport-trace",
		"radar.traces.processor": "simple",
	})

	var gotTraceparent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotTraceparent = req.Header.Get("traceparent")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// 上游 span
	ctx, parent := r.Tracer("test").Start(context.Background(), "parent-op")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api", nil)
	if err != nil {
		t.Fatal(err)
	}

	client := &http.Client{Transport: NewTransport(nil, r)}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	parent.End()

	// 服务端收到 traceparent，且与上游同一条 trace
	if gotTraceparent == "" {
		t.Fatal("traceparent header missing")
	}
	if !strings.Contains(gotTraceparent, parent.TraceID()) {
		t.Fatalf("traceparent %q does not carry parent trace %s", gotTraceparent, parent.TraceID())
	}

	spans := inmem.GetSpans()
	var clientSpan *tracetest.SpanStub
	for i := range spans {
		if spans[i].Name == "GET "+srv.Listener.Addr().String() {
			clientSpan = &spans[i]
		}
	}
	if clientSpan == nil {
		t.Fatalf("client span not found: %v", spans)
	}
	if clientSpan.SpanKind != trace.SpanKindClient {
		t.Fatalf("kind = %v", clientSpan.SpanKind)
	}
}

func TestTransport_NilBase(t *testing.T) {
	r := newTestRadar(t, map[string]any{})
	rt := NewTransport(nil, r)
	if rt == nil {
		t.Fatal("NewTransport must not return nil")
	}
}
