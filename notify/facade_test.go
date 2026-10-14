package notify

import (
	"context"
	"testing"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/foundation"
	"github.com/zhoudm1743/go-fast-framework/mock"
)

// fakeLog 记录式 contracts.Log 实现：链式方法返回自身（字段就地合并），便于断言。
type fakeLog struct {
	entries []fakeEntry
	fields  map[string]any
}

type fakeEntry struct {
	level  string
	msg    string
	fields map[string]any
}

func newFakeLog() *fakeLog {
	return &fakeLog{fields: map[string]any{}}
}

func (f *fakeLog) add(level string, args ...any) {
	msg := ""
	if len(args) > 0 {
		if s, ok := args[0].(string); ok {
			msg = s
		}
	}
	cp := make(map[string]any, len(f.fields))
	for k, v := range f.fields {
		cp[k] = v
	}
	f.entries = append(f.entries, fakeEntry{level: level, msg: msg, fields: cp})
}

func (f *fakeLog) Debug(args ...any)                 { f.add("debug", args...) }
func (f *fakeLog) Debugf(string, ...any)            {}
func (f *fakeLog) Info(args ...any)                  { f.add("info", args...) }
func (f *fakeLog) Infof(string, ...any)             {}
func (f *fakeLog) Warn(args ...any)                  { f.add("warn", args...) }
func (f *fakeLog) Warnf(string, ...any)             {}
func (f *fakeLog) Error(args ...any)                 { f.add("error", args...) }
func (f *fakeLog) Errorf(string, ...any)            {}
func (f *fakeLog) Fatal(args ...any)                 { f.add("fatal", args...) }
func (f *fakeLog) Fatalf(string, ...any)            {}
func (f *fakeLog) Panic(args ...any)                 { f.add("panic", args...) }
func (f *fakeLog) Panicf(string, ...any)            {}
func (f *fakeLog) WithContext(context.Context) contracts.Log { return f }

func (f *fakeLog) WithField(key string, value any) contracts.Log {
	f.fields[key] = value
	return f
}

func (f *fakeLog) WithFields(fields map[string]any) contracts.Log {
	for k, v := range fields {
		f.fields[k] = v
	}
	return f
}

func (f *fakeLog) WithError(err error) contracts.Log {
	f.fields["error"] = err
	return f
}

func TestLogNotify_ForwardsLevels(t *testing.T) {
	fl := newFakeLog()
	ln := newLogNotify(fl)

	ln.Debug("d")
	ln.Info("i")
	ln.Warn("w")
	ln.Error("e")

	want := []struct {
		level string
		msg   string
	}{
		{"debug", "d"},
		{"info", "i"},
		{"warn", "w"},
		{"error", "e"},
	}
	if len(fl.entries) != len(want) {
		t.Fatalf("entries = %d, want %d", len(fl.entries), len(want))
	}
	for i, w := range want {
		if fl.entries[i].level != w.level || fl.entries[i].msg != w.msg {
			t.Fatalf("entry %d = %+v, want %s %q", i, fl.entries[i], w.level, w.msg)
		}
	}
}

func TestLogNotify_WithFieldChains(t *testing.T) {
	fl := newFakeLog()
	ln := newLogNotify(fl).WithField("channel", "notify").WithField("extra", 1)

	if _, ok := ln.(contracts.LogNotify); !ok {
		t.Fatal("WithField must return contracts.LogNotify")
	}
	ln.Info("hello")

	if len(fl.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(fl.entries))
	}
	fields := fl.entries[0].fields
	if fields["channel"] != "notify" || fields["extra"] != 1 {
		t.Fatalf("fields = %v", fields)
	}
}

func TestLogNotify_WithError(t *testing.T) {
	fl := newFakeLog()
	ln := newLogNotify(fl).WithError(errTest)
	ln.Error("boom")

	fields := fl.entries[0].fields
	if fields["error"] != errTest {
		t.Fatalf("error field = %v", fields)
	}
}

var errTest = &testError{"test error"}

type testError struct{ s string }

func (e *testError) Error() string { return e.s }

// newFacadeTestAppWithQueue 装配一个可运行的最小应用：config/log/queue 实例 + notify Provider。
func newFacadeTestAppWithQueue(t *testing.T) (foundation.Application, *mock.MockQueue) {
	t.Helper()
	app := foundation.NewApplication(".")
	app.Instance("config", newTestConfig(map[string]any{}))
	app.Instance("log", mock.NewMockLog())
	mq := mock.NewMockQueue()
	app.Instance("queue", mq)
	app.SetProviders([]foundation.ServiceProvider{&ServiceProvider{}})
	app.Boot()
	return app, mq
}

func newFacadeTestApp(t *testing.T) foundation.Application {
	t.Helper()
	app, _ := newFacadeTestAppWithQueue(t)
	return app
}

func TestNotifyFacade_ConstructsBuilders(t *testing.T) {
	app := newFacadeTestApp(t)
	n := app.MustMake("notify").(contracts.Notify)

	if _, ok := n.Mail().(*mailMessage); !ok {
		t.Fatalf("Mail() = %T", n.Mail())
	}
	if _, ok := n.Webhook("https://example.com").(*webhookMessage); !ok {
		t.Fatalf("Webhook() = %T", n.Webhook("https://example.com"))
	}
	ln := n.Log()
	if _, ok := ln.(*logNotify); !ok {
		t.Fatalf("Log() = %T", ln)
	}
}

func TestNotifyFacade_SharedHTTPClient(t *testing.T) {
	app := newFacadeTestApp(t)
	n := app.MustMake("notify").(contracts.Notify)

	w1 := n.Webhook("https://a.example.com").(*webhookMessage)
	w2 := n.Webhook("https://b.example.com").(*webhookMessage)
	if w1.client == nil || w1.client != w2.client {
		t.Fatal("webhook builders must share one HTTP client from the facade")
	}
}

func TestNotifyFacade_DispatchEndToEnd(t *testing.T) {
	app, mq := newFacadeTestAppWithQueue(t)
	n := app.MustMake("notify").(contracts.Notify)

	err := n.Mail().To("a@example.com").Subject("s").Text("b").Dispatch()
	if err != nil {
		t.Fatalf("Dispatch() error: %v", err)
	}
	if len(mq.Jobs) != 1 {
		t.Fatalf("expected 1 queued job, got %d", len(mq.Jobs))
	}
	if mq.Jobs[0].Job.Signature() != "go-fast:notify" {
		t.Fatalf("signature = %q", mq.Jobs[0].Job.Signature())
	}
}
