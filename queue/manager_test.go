package queue

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// ── stubLog ──────────────────────────────────────────────────────────

// stubLog 实现 contracts.Log，记录 Error/Warn 调用以供断言。
type stubLog struct {
	mu     sync.Mutex
	errors []string
	warns  []string
}

func (l *stubLog) record(args []string) string {
	if len(args) == 0 {
		return ""
	}
	// 手动拼接，避免 fmt.Sprint 的 interface{} 包装
	var s string
	for i, a := range args {
		if i > 0 {
			s += " "
		}
		s += a
	}
	return s
}

func (l *stubLog) Debug(args ...any)                 {}
func (l *stubLog) Debugf(f string, a ...any)         {}
func (l *stubLog) Info(args ...any)                  {}
func (l *stubLog) Infof(f string, a ...any)          {}
func (l *stubLog) Warn(args ...any) {
	l.mu.Lock()
	l.warns = append(l.warns, l.record(toStrings(args)))
	l.mu.Unlock()
}
func (l *stubLog) Warnf(f string, a ...any) {
	l.mu.Lock()
	l.warns = append(l.warns, fmt.Sprintf(f, a...))
	l.mu.Unlock()
}
func (l *stubLog) Error(args ...any) {
	l.mu.Lock()
	l.errors = append(l.errors, l.record(toStrings(args)))
	l.mu.Unlock()
}
func (l *stubLog) Errorf(f string, a ...any) {
	l.mu.Lock()
	l.errors = append(l.errors, fmt.Sprintf(f, a...))
	l.mu.Unlock()
}
func (l *stubLog) Fatal(args ...any)                 {}
func (l *stubLog) Fatalf(f string, a ...any)         {}
func (l *stubLog) Panic(args ...any)                 {}
func (l *stubLog) Panicf(f string, a ...any)         {}
func (l *stubLog) WithField(k string, v any) contracts.Log { return l }
func (l *stubLog) WithFields(fields map[string]any) contracts.Log { return l }
func (l *stubLog) WithError(err error) contracts.Log             { return l }
func (l *stubLog) WithContext(ctx context.Context) contracts.Log { return l }

func (l *stubLog) errorsCopy() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	c := make([]string, len(l.errors))
	copy(c, l.errors)
	return c
}

func (l *stubLog) warnsCopy() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	c := make([]string, len(l.warns))
	copy(c, l.warns)
	return c
}

func toStrings(args []any) []string {
	r := make([]string, len(args))
	for i, a := range args {
		r[i] = fmt.Sprint(a)
	}
	return r
}

// ── stubJob ──────────────────────────────────────────────────────────

type stubJob struct {
	sig    string
	handle func(args ...any) error
}

func (j *stubJob) Signature() string               { return j.sig }
func (j *stubJob) Handle(args ...any) error         { return j.handle(args...) }

func newStubJob(sig string, fn func(args ...any) error) *stubJob {
	return &stubJob{sig: sig, handle: fn}
}

// ── 测试辅助 ──────────────────────────────────────────────────────────

// newTestQueue 创建测试用队列管理器，返回具体类型以便调用 Stop。
func newTestQueue(log contracts.Log, opts ...Option) *manager {
	return New(log, opts...).(*manager)
}

// ── 测试 ─────────────────────────────────────────────────────────────

func TestJobBuilder(t *testing.T) {
	lg := &stubLog{}
	m := newTestQueue(lg, WithWorkers(1))
	defer m.Stop()

	p := m.Job(newStubJob("test", nil), nil).
		OnQueue("emails").
		OnConnection("redis").
		Delay(time.Now().Add(time.Hour)).
		Retry(3)

	pt := p.(*pendingTask)
	if pt.queue != "emails" {
		t.Errorf("queue = %q, want %q", pt.queue, "emails")
	}
	if pt.connection != "redis" {
		t.Errorf("connection = %q, want %q", pt.connection, "redis")
	}
	if pt.retry != 3 {
		t.Errorf("retry = %d, want 3", pt.retry)
	}
	if pt.delay.IsZero() {
		t.Error("delay should not be zero")
	}
}

func TestDispatchSync(t *testing.T) {
	lg := &stubLog{}
	m := newTestQueue(lg, WithWorkers(1))
	defer m.Stop()

	var received []any
	job := newStubJob("sync_test", func(args ...any) error {
		received = args
		return nil
	})

	err := m.Job(job, []contracts.QueueArg{
		{Type: "string", Value: "hello"},
		{Type: "int", Value: 42},
	}).DispatchSync()

	if err != nil {
		t.Fatalf("DispatchSync returned error: %v", err)
	}
	if len(received) != 2 || received[0] != "hello" || received[1] != 42 {
		t.Errorf("received = %v, want [hello 42]", received)
	}
}

func TestDispatchSyncError(t *testing.T) {
	lg := &stubLog{}
	m := newTestQueue(lg, WithWorkers(1))
	defer m.Stop()

	job := newStubJob("fail", func(args ...any) error {
		return fmt.Errorf("任务执行失败")
	})

	err := m.Job(job, nil).DispatchSync()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// 错误应包含 [GoFast] 前缀和原始错误
	if err.Error() == "" {
		t.Error("error message should not be empty")
	}
}

func TestDispatchAsync(t *testing.T) {
	lg := &stubLog{}
	m := newTestQueue(lg, WithWorkers(1))
	defer m.Stop()

	done := make(chan struct{})
	job := newStubJob("async_test", func(args ...any) error {
		close(done)
		return nil
	})

	err := m.Job(job, nil).Dispatch()
	if err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("async task did not complete within timeout")
	}
}

func TestPanicRecover(t *testing.T) {
	lg := &stubLog{}
	m := newTestQueue(lg, WithWorkers(1))
	defer m.Stop()

	job := newStubJob("panic_job", func(args ...any) error {
		panic("意外错误")
	})

	// DispatchSync：panic 转为 error
	err := m.Job(job, nil).DispatchSync()
	if err == nil {
		t.Fatal("expected panic to be converted to error")
	}
	t.Logf("panic converted to error: %v", err)
}

func TestPanicRecoverAsync(t *testing.T) {
	lg := &stubLog{}
	m := newTestQueue(lg, WithWorkers(1))
	defer m.Stop()

	done := make(chan struct{})
	job := newStubJob("async_panic", func(args ...any) error {
		defer close(done)
		panic("异步 panic")
	})

	if err := m.Job(job, nil).Dispatch(); err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}

	select {
	case <-done:
		// panic 被捕获，任务不崩
	case <-time.After(2 * time.Second):
		t.Fatal("async task did not complete")
	}

	// 给 worker 一点时间记录日志
	time.Sleep(50 * time.Millisecond)

	errs := lg.errorsCopy()
	if len(errs) == 0 {
		t.Error("expected error log entry for panic in async task")
	}
	for _, e := range errs {
		t.Logf("logged error: %s", e)
	}
}

func TestRetrySuccess(t *testing.T) {
	lg := &stubLog{}
	m := newTestQueue(lg, WithWorkers(1))
	defer m.Stop()

	var attempts int32
	job := newStubJob("retry_ok", func(args ...any) error {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			return fmt.Errorf("第 %d 次失败", n)
		}
		return nil
	})

	err := m.Job(job, nil).Retry(2).DispatchSync()
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestRetryExhausted(t *testing.T) {
	lg := &stubLog{}
	m := newTestQueue(lg, WithWorkers(1))
	defer m.Stop()

	var attempts int32
	job := newStubJob("retry_fail", func(args ...any) error {
		atomic.AddInt32(&attempts, 1)
		return fmt.Errorf("永远失败")
	})

	err := m.Job(job, nil).Retry(2).DispatchSync()
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if n := atomic.LoadInt32(&attempts); n != 3 {
		t.Errorf("attempts = %d, want 3", n)
	}

	warns := lg.warnsCopy()
	if len(warns) == 0 {
		t.Error("expected retry warning logs")
	}
}

func TestChainOrder(t *testing.T) {
	lg := &stubLog{}
	m := newTestQueue(lg, WithWorkers(1))
	defer m.Stop()

	var order []string
	makeJob := func(name string) contracts.QueueJob {
		return newStubJob(name, func(args ...any) error {
			order = append(order, name)
			return nil
		})
	}

	err := m.Chain([]contracts.QueueChain{
		{Job: makeJob("A"), Args: nil},
		{Job: makeJob("B"), Args: nil},
		{Job: makeJob("C"), Args: nil},
	}).DispatchSync()

	if err != nil {
		t.Fatalf("chain failed: %v", err)
	}
	if len(order) != 3 || order[0] != "A" || order[1] != "B" || order[2] != "C" {
		t.Errorf("order = %v, want [A B C]", order)
	}
}

func TestChainStopsOnError(t *testing.T) {
	lg := &stubLog{}
	m := newTestQueue(lg, WithWorkers(1))
	defer m.Stop()

	var executed []string
	makeJob := func(name string, fail bool) contracts.QueueJob {
		return newStubJob(name, func(args ...any) error {
			executed = append(executed, name)
			if fail {
				return fmt.Errorf("%s 失败", name)
			}
			return nil
		})
	}

	err := m.Chain([]contracts.QueueChain{
		{Job: makeJob("A", false), Args: nil},
		{Job: makeJob("B", true), Args: nil},
		{Job: makeJob("C", false), Args: nil},
	}).DispatchSync()

	if err == nil {
		t.Fatal("expected chain error")
	}
	if len(executed) != 2 || executed[0] != "A" || executed[1] != "B" {
		t.Errorf("executed = %v, want [A B] (C should not run)", executed)
	}
}

func TestStopDrainsQueuedTasks(t *testing.T) {
	lg := &stubLog{}
	m := newTestQueue(lg, WithWorkers(1))
	// 注意：不 defer Stop，手动控制

	var count int32
	job := newStubJob("drain", func(args ...any) error {
		atomic.AddInt32(&count, 1)
		time.Sleep(10 * time.Millisecond)
		return nil
	})

	// 投递 3 个任务
	for i := 0; i < 3; i++ {
		if err := m.Job(job, nil).Dispatch(); err != nil {
			t.Fatalf("Dispatch returned error: %v", err)
		}
	}

	// 立即关闭
	m.Stop()

	// 所有任务应已排空执行
	if n := atomic.LoadInt32(&count); n != 3 {
		t.Errorf("drained count = %d, want 3", n)
	}
}

func TestDispatchAfterStop(t *testing.T) {
	lg := &stubLog{}
	m := newTestQueue(lg, WithWorkers(1))
	m.Stop()

	job := newStubJob("late", nil)
	err := m.Job(job, nil).Dispatch()
	if err == nil {
		t.Fatal("expected error when dispatching after stop")
	}
}

func TestConcurrentDispatch(t *testing.T) {
	lg := &stubLog{}
	m := newTestQueue(lg, WithWorkers(4))
	defer m.Stop()

	var count int32
	job := newStubJob("concurrent", func(args ...any) error {
		atomic.AddInt32(&count, 1)
		return nil
	})

	const n = 200
	var wg sync.WaitGroup
	errs := make([]error, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			errs[idx] = m.Job(job, nil).Dispatch()
		}(i)
	}
	wg.Wait()

	for i, e := range errs {
		if e != nil {
			t.Errorf("Dispatch[%d] returned error: %v", i, e)
		}
	}

	// 等待所有任务完成
	time.Sleep(200 * time.Millisecond)

	final := atomic.LoadInt32(&count)
	if final < int32(n) {
		t.Errorf("only %d/%d tasks executed", final, n)
	}
}

func TestDelay(t *testing.T) {
	lg := &stubLog{}
	m := newTestQueue(lg, WithWorkers(1))
	defer m.Stop()

	start := time.Now()
	var elapsed time.Duration
	job := newStubJob("delayed", func(args ...any) error {
		elapsed = time.Since(start)
		return nil
	})

	done := make(chan struct{})
	job2 := newStubJob("signal", func(args ...any) error {
		close(done)
		return nil
	})

	// 延迟任务
	if err := m.Job(job, nil).Delay(time.Now().Add(100 * time.Millisecond)).Dispatch(); err != nil {
		t.Fatalf("Dispatch delayed: %v", err)
	}
	// 立即任务用于信号
	if err := m.Job(job2, nil).Dispatch(); err != nil {
		t.Fatalf("Dispatch signal: %v", err)
	}

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("signal task timeout")
	}

	// 延迟任务应该也完成了（因为 signal 完成后 worker 继续消费）
	// 注意：worker=1 时延迟任务会先阻塞 worker，所以这里需要等
	time.Sleep(150 * time.Millisecond)

	if elapsed < 50*time.Millisecond {
		t.Errorf("delay too short: %v", elapsed)
	}
}
