package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/zhoudm1743/go-fast-framework/facades"
	"github.com/zhoudm1743/go-fast-framework/foundation"
	"github.com/zhoudm1743/go-fast-framework/mock"
)

// recordedRequest 记录一次收到的 webhook 请求。
type recordedRequest struct {
	method string
	header http.Header
	body   []byte
}

// webhookRecorder 并发安全地记录请求并返回预设响应序列。
type webhookRecorder struct {
	mu        sync.Mutex
	requests  []recordedRequest
	responses []int // 按请求顺序消费；超出最后一个的用最后一个
}

func (r *webhookRecorder) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		_ = req.Body.Close()
		r.mu.Lock()
		r.requests = append(r.requests, recordedRequest{method: req.Method, header: req.Header.Clone(), body: body})
		n := len(r.requests)
		code := r.responses[len(r.responses)-1]
		if n <= len(r.responses) {
			code = r.responses[n-1]
		}
		r.mu.Unlock()
		w.WriteHeader(code)
	}
}

func (r *webhookRecorder) snapshot() []recordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]recordedRequest, len(r.requests))
	copy(out, r.requests)
	return out
}

var idKeyPattern = regexp.MustCompile(`^[0-9a-hjkmnpqrstvwxyz]{16}$`)

func newWebhookForTest(cfgMap map[string]any, url string) *webhookMessage {
	cfg := newTestConfig(cfgMap)
	return newWebhookMessage(cfg, mock.NewMockLog(), &http.Client{Timeout: 2 * time.Second}, url)
}

func TestWebhookSend_Success(t *testing.T) {
	rec := &webhookRecorder{responses: []int{200}}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	w := newWebhookForTest(map[string]any{}, srv.URL).
		Header("X-Custom", "v1").
		Body([]byte("raw body"))

	if err := w.Send(); err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	reqs := rec.snapshot()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
	if reqs[0].method != http.MethodPost {
		t.Fatalf("method = %s, want POST", reqs[0].method)
	}
	if string(reqs[0].body) != "raw body" {
		t.Fatalf("body = %q", reqs[0].body)
	}
	if reqs[0].header.Get("X-Custom") != "v1" {
		t.Fatal("custom header missing")
	}
	key := reqs[0].header.Get("Idempotency-Key")
	if !idKeyPattern.MatchString(key) {
		t.Fatalf("auto idempotency key %q not a valid framework id", key)
	}
}

func TestWebhookSend_JSONBody(t *testing.T) {
	rec := &webhookRecorder{responses: []int{200}}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	w := newWebhookForTest(map[string]any{}, srv.URL).
		JSON(map[string]any{"msg": "hello"})

	if err := w.Send(); err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	reqs := rec.snapshot()
	if ct := reqs[0].header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q", ct)
	}
	var decoded map[string]any
	if err := json.Unmarshal(reqs[0].body, &decoded); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}
	if decoded["msg"] != "hello" {
		t.Fatalf("body = %s", reqs[0].body)
	}
}

func TestWebhookSend_4xxNoRetry(t *testing.T) {
	rec := &webhookRecorder{responses: []int{400}}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	w := newWebhookForTest(map[string]any{"notify.webhook.retry": 3}, srv.URL).
		Body([]byte("x"))
	if err := w.Send(); err == nil {
		t.Fatal("expected error on 400")
	}
	if n := len(rec.snapshot()); n != 1 {
		t.Fatalf("400 must not retry, got %d requests", n)
	}
}

func TestWebhookSend_5xxRetryThenSuccess(t *testing.T) {
	rec := &webhookRecorder{responses: []int{500, 500, 200}}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	w := newWebhookForTest(map[string]any{
		"notify.webhook.retry":         3,
		"notify.webhook.retry_interval": "1ms",
	}, srv.URL).Body([]byte("x"))

	if err := w.Send(); err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	reqs := rec.snapshot()
	if len(reqs) != 3 {
		t.Fatalf("expected 3 attempts, got %d", len(reqs))
	}
	key := reqs[0].header.Get("Idempotency-Key")
	for i, r := range reqs {
		if r.header.Get("Idempotency-Key") != key {
			t.Fatalf("attempt %d idempotency key %q != first %q", i, r.header.Get("Idempotency-Key"), key)
		}
	}
}

func TestWebhookSend_RetryExhausted(t *testing.T) {
	rec := &webhookRecorder{responses: []int{500}}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	start := time.Now()
	w := newWebhookForTest(map[string]any{
		"notify.webhook.retry":         2,
		"notify.webhook.retry_interval": "30ms",
	}, srv.URL).Body([]byte("x"))

	if err := w.Send(); err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	if n := len(rec.snapshot()); n != 3 { // 首次 + 2 次重试
		t.Fatalf("expected 3 attempts, got %d", n)
	}
	if elapsed := time.Since(start); elapsed < 30*time.Millisecond {
		t.Fatalf("backoff not respected: elapsed %v", elapsed)
	}
}

func TestWebhookSend_CustomIdempotencyKey(t *testing.T) {
	rec := &webhookRecorder{responses: []int{200}}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	w := newWebhookForTest(map[string]any{}, srv.URL).
		IdempotencyKey("order-123").
		Body([]byte("x"))
	if err := w.Send(); err != nil {
		t.Fatal(err)
	}
	if key := rec.snapshot()[0].header.Get("Idempotency-Key"); key != "order-123" {
		t.Fatalf("idempotency key = %q", key)
	}
}

func TestWebhookMessage_BodyOverrideOrder(t *testing.T) {
	rec := &webhookRecorder{responses: []int{200, 200}}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	// JSON 后设 Body：body 生效，自动 Content-Type 清除
	w := newWebhookForTest(map[string]any{}, srv.URL).
		JSON(map[string]any{"a": 1}).
		Body([]byte("raw"))
	if err := w.Send(); err != nil {
		t.Fatal(err)
	}
	reqs := rec.snapshot()
	if string(reqs[0].body) != "raw" {
		t.Fatalf("body = %q", reqs[0].body)
	}
	if ct := reqs[0].header.Get("Content-Type"); ct == "application/json" {
		t.Fatal("stale application/json content-type should be cleared when Body overrides JSON")
	}

	// Body 后设 JSON：JSON 生效
	w = newWebhookForTest(map[string]any{}, srv.URL).
		Body([]byte("raw")).
		JSON(map[string]any{"a": 1})
	if err := w.Send(); err != nil {
		t.Fatal(err)
	}
	reqs = rec.snapshot()
	if err := json.Unmarshal(reqs[1].body, new(map[string]any)); err != nil {
		t.Fatalf("second request body should be JSON, got %q", reqs[1].body)
	}
}

func TestWebhookSend_EmptyURL(t *testing.T) {
	w := newWebhookForTest(map[string]any{}, "")
	if err := w.Body([]byte("x")).Send(); err == nil {
		t.Fatal("expected error on empty URL")
	}
}

func TestWebhookDispatch_EnqueuesPayload(t *testing.T) {
	_, mq := setupDispatchApp(t)
	srv := httptest.NewServer((&webhookRecorder{responses: []int{200}}).handler())
	defer srv.Close()

	w := newWebhookForTest(map[string]any{}, srv.URL).
		Header("X-A", "1").
		Body([]byte("payload"))
	if err := w.Dispatch(); err != nil {
		t.Fatalf("Dispatch() error: %v", err)
	}
	if len(mq.Jobs) != 1 {
		t.Fatalf("expected 1 queued job, got %d", len(mq.Jobs))
	}
	p, ok := mq.Jobs[0].Args[1].Value.(webhookPayload)
	if !ok {
		t.Fatalf("payload type = %T", mq.Jobs[0].Args[1].Value)
	}
	if p.URL != srv.URL || string(p.Body) != "payload" || p.Headers["X-A"] != "1" {
		t.Fatalf("unexpected payload: %+v", p)
	}
	if !idKeyPattern.MatchString(p.IdempotencyKey) {
		t.Fatalf("payload idempotency key %q should be auto-generated", p.IdempotencyKey)
	}
}

func TestWebhookPayload_JSONRoundTrip(t *testing.T) {
	p := webhookPayload{
		URL:            "https://example.com/hook",
		Headers:        map[string]string{"Authorization": "Bearer x"},
		Body:           []byte("bin\x00ary"),
		IdempotencyKey: "k1",
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]any
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeWebhookPayload(generic)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, p) {
		t.Fatalf("round trip mismatch: %+v vs %+v", decoded, p)
	}
}

func TestNotifyJobHandle_WebhookPayload(t *testing.T) {
	rec := &webhookRecorder{responses: []int{200}}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	app := foundation.NewApplication(".")
	app.Instance("config", newTestConfig(map[string]any{}))
	app.Instance("log", mock.NewMockLog())
	facades.SetApp(app)

	err := (notifyJob{}).Handle("webhook", webhookPayload{
		URL:            srv.URL,
		Body:           []byte("async"),
		IdempotencyKey: "async-key",
	})
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	reqs := rec.snapshot()
	if len(reqs) != 1 || reqs[0].header.Get("Idempotency-Key") != "async-key" {
		t.Fatalf("unexpected requests: %+v", reqs)
	}
}
