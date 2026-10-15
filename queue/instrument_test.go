package queue

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// ── radar 埋点测试桩 ─────────────────────────────────────────────────

type stubRadarTracer struct {
	mu     sync.Mutex
	spans  []string
	span   *stubRadarSpan
}

func (t *stubRadarTracer) Start(ctx context.Context, name string, opts ...contracts.RadarSpanOption) (context.Context, contracts.RadarSpan) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.spans = append(t.spans, name)
	t.span = &stubRadarSpan{}
	return ctx, t.span
}

type stubRadarSpan struct {
	ended    bool
	errRecorded bool
	statusSet contracts.RadarStatusCode
}

func (s *stubRadarSpan) End()                                       { s.ended = true }
func (s *stubRadarSpan) SetAttributes(...contracts.RadarAttr)       {}
func (s *stubRadarSpan) AddEvent(string, ...contracts.RadarAttr)    {}
func (s *stubRadarSpan) RecordError(error)                          { s.errRecorded = true }
func (s *stubRadarSpan) SetStatus(c contracts.RadarStatusCode, _ string) { s.statusSet = c }
func (s *stubRadarSpan) TraceID() string                            { return "trace" }
func (s *stubRadarSpan) SpanID() string                             { return "span" }

type jobOK struct{ done chan struct{} }

func (j jobOK) Signature() string { return "test:ok" }
func (j jobOK) Handle(...any) error {
	close(j.done)
	return nil
}

type jobFail struct{ done chan struct{} }

func (j jobFail) Signature() string { return "test:fail" }
func (j jobFail) Handle(...any) error {
	close(j.done)
	return errors.New("boom")
}

func waitCh(t *testing.T, ch chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("job not executed in time")
	}
}

func TestQueueWorkerSpan_Success(t *testing.T) {
	tracer := &stubRadarTracer{}
	q := New(&stubLog{}, WithWorkers(1)).(*manager)
	q.SetTracer(tracer)
	defer q.Stop()

	j := jobOK{done: make(chan struct{})}
	if err := q.Job(j, nil).Dispatch(); err != nil {
		t.Fatal(err)
	}
	waitCh(t, j.done)
	// 等待 worker 完成 span.End
	time.Sleep(50 * time.Millisecond)

	if len(tracer.spans) != 1 || tracer.spans[0] != "queue.exec test:ok" {
		t.Fatalf("spans = %v", tracer.spans)
	}
	if tracer.span == nil || !tracer.span.ended {
		t.Fatal("span must be ended")
	}
	if tracer.span.errRecorded {
		t.Fatal("no error should be recorded on success")
	}
}

func TestQueueWorkerSpan_Failure(t *testing.T) {
	tracer := &stubRadarTracer{}
	q := New(&stubLog{}, WithWorkers(1)).(*manager)
	q.SetTracer(tracer)
	defer q.Stop()

	j := jobFail{done: make(chan struct{})}
	if err := q.Job(j, nil).Dispatch(); err != nil {
		t.Fatal(err)
	}
	waitCh(t, j.done)
	time.Sleep(50 * time.Millisecond)

	if tracer.span == nil || !tracer.span.errRecorded {
		t.Fatal("error must be recorded on span")
	}
	if tracer.span.statusSet != contracts.RadarStatusError {
		t.Fatal("span status must be error")
	}
}

func TestQueueWorkerSpan_NoTracer(t *testing.T) {
	// 未挂载 tracer 时正常执行（不 panic）
	q := New(&stubLog{}, WithWorkers(1)).(*manager)
	defer q.Stop()

	j := jobOK{done: make(chan struct{})}
	if err := q.Job(j, nil).Dispatch(); err != nil {
		t.Fatal(err)
	}
	waitCh(t, j.done)
}
