package notify

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/facades"
	"github.com/zhoudm1743/go-fast-framework/foundation"
	"github.com/zhoudm1743/go-fast-framework/mock"
)

// setupDispatchApp 构造一个注入了 mock queue 的应用并设为全局。
func setupDispatchApp(t *testing.T) (foundation.Application, *mock.MockQueue) {
	t.Helper()
	app := foundation.NewApplication(".")
	mq := mock.NewMockQueue()
	app.Instance("queue", mq)
	facades.SetApp(app)
	return app, mq
}

func TestMailDispatch_EnqueuesPayload(t *testing.T) {
	_, mq := setupDispatchApp(t)
	cfg := newTestConfig(map[string]any{})
	log := mock.NewMockLog()

	m := newMailMessage(cfg, log).
		To("a@example.com").
		Subject("s").
		Text("b").
		From("me@example.com")

	if err := m.Dispatch(); err != nil {
		t.Fatalf("Dispatch() error: %v", err)
	}
	if len(mq.Jobs) != 1 {
		t.Fatalf("expected 1 queued job, got %d", len(mq.Jobs))
	}
	job := mq.Jobs[0]
	if job.Job.Signature() != "go-fast:notify" {
		t.Fatalf("unexpected signature %q", job.Job.Signature())
	}
	if len(job.Args) != 2 || job.Args[0].Value != "mail" {
		t.Fatalf("unexpected args: %+v", job.Args)
	}
	p, ok := job.Args[1].Value.(mailPayload)
	if !ok {
		t.Fatalf("payload type = %T, want mailPayload", job.Args[1].Value)
	}
	if p.To[0] != "a@example.com" || p.Subject != "s" || p.From != "me@example.com" {
		t.Fatalf("unexpected payload: %+v", p)
	}
}

func TestMailDispatch_ValidatesBeforeEnqueue(t *testing.T) {
	_, mq := setupDispatchApp(t)
	cfg := newTestConfig(map[string]any{})

	m := newMailMessage(cfg, mock.NewMockLog()).Subject("s").Text("b")
	if err := m.Dispatch(); err == nil || !strings.Contains(err.Error(), "收件人") {
		t.Fatalf("expected 收件人 validation error, got %v", err)
	}
	if len(mq.Jobs) != 0 {
		t.Fatal("no job should be enqueued on validation failure")
	}
}

// recordingPending 记录 Dispatch 选项的 QueuePending。
type recordingPending struct {
	queue string
	delay time.Time
	retry int
	calls int
}

func (p *recordingPending) OnQueue(q string) contracts.QueuePending   { p.queue = q; return p }
func (p *recordingPending) OnConnection(string) contracts.QueuePending { return p }
func (p *recordingPending) Delay(d time.Time) contracts.QueuePending    { p.delay = d; return p }
func (p *recordingPending) Retry(n int) contracts.QueuePending          { p.retry = n; return p }
func (p *recordingPending) Dispatch() error                             { p.calls++; return nil }
func (p *recordingPending) DispatchSync() error                         { p.calls++; return nil }

func TestDispatch_OptionsPropagation(t *testing.T) {
	app := foundation.NewApplication(".")
	rec := &recordingPending{}
	mq := mock.NewMockQueue()
	mq.JobFunc = func(job contracts.QueueJob, args []contracts.QueueArg) contracts.QueuePending {
		return rec
	}
	app.Instance("queue", mq)
	facades.SetApp(app)

	cfg := newTestConfig(map[string]any{})
	delay := time.Now().Add(time.Minute)
	m := newMailMessage(cfg, mock.NewMockLog()).To("a@example.com").Subject("s").Text("b")

	err := m.Dispatch(
		contracts.WithQueue("high"),
		contracts.WithDelay(delay),
		contracts.WithRetry(2),
	)
	if err != nil {
		t.Fatalf("Dispatch() error: %v", err)
	}
	if rec.calls != 1 || rec.queue != "high" || rec.retry != 2 {
		t.Fatalf("unexpected pending: %+v", rec)
	}
	if !rec.delay.Equal(delay) {
		t.Fatalf("delay = %v, want %v", rec.delay, delay)
	}
}

func TestMailPayload_JSONRoundTrip(t *testing.T) {
	p := mailPayload{
		From:    "me@example.com",
		To:      []string{"a@example.com"},
		Subject: "订单通知",
		Text:    "正文",
		HTML:    "<p>正文</p>",
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	// 模拟 redis driver 的 JSON 往返：进入 map 后再还原
	var generic map[string]any
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeMailPayload(generic)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, p) {
		t.Fatalf("round trip mismatch: %+v vs %+v", decoded, p)
	}
}

func TestNotifyJobHandle_StructPayload(t *testing.T) {
	app := foundation.NewApplication(".")
	app.Instance("config", newTestConfig(map[string]any{}))
	app.Instance("log", mock.NewMockLog())
	facades.SetApp(app)

	// SMTP 未配置：能走到 SMTP 校验说明 payload 解码与依赖解析成功
	err := (notifyJob{}).Handle("mail", mailPayload{To: []string{"a@example.com"}, Subject: "s", Text: "b"})
	if err == nil || !strings.Contains(err.Error(), "SMTP") {
		t.Fatalf("expected SMTP config error, got %v", err)
	}
}

func TestNotifyJobHandle_MapPayload(t *testing.T) {
	app := foundation.NewApplication(".")
	app.Instance("config", newTestConfig(map[string]any{}))
	app.Instance("log", mock.NewMockLog())
	facades.SetApp(app)

	// redis 形态：payload 经 JSON 往返后是 map[string]any
	data, _ := json.Marshal(mailPayload{To: []string{"a@example.com"}, Subject: "s", Text: "b"})
	var generic map[string]any
	_ = json.Unmarshal(data, &generic)

	err := (notifyJob{}).Handle("mail", generic)
	if err == nil || !strings.Contains(err.Error(), "SMTP") {
		t.Fatalf("expected SMTP config error after map decode, got %v", err)
	}
}

func TestNotifyJobHandle_UnknownKind(t *testing.T) {
	if err := (notifyJob{}).Handle("sms", map[string]any{}); err == nil ||
		!strings.Contains(err.Error(), "未知消息通道") {
		t.Fatalf("expected unknown channel error, got %v", err)
	}
}
