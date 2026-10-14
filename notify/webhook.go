package notify

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/facades"
	"github.com/zhoudm1743/go-fast-framework/id"
)

const (
	cfgWebhookTimeout       = "notify.webhook.timeout"
	cfgWebhookRetry         = "notify.webhook.retry"
	cfgWebhookRetryInterval = "notify.webhook.retry_interval"
)

// webhookMessage 实现 contracts.WebhookMessage。
type webhookMessage struct {
	cfg     contracts.Config
	log     contracts.Log
	client  *http.Client // nil 时按配置惰性构造
	url     string
	headers map[string]string
	body    []byte
	autoCT  bool // body 是否来自 JSON()（自动设置了 application/json）
	jsonErr error

	idemKey       string
	retry         *int
	retryInterval *time.Duration
}

// newWebhookMessage 创建 Webhook 消息构造器。
func newWebhookMessage(cfg contracts.Config, log contracts.Log, client *http.Client, url string) *webhookMessage {
	return &webhookMessage{cfg: cfg, log: log, client: client, url: url}
}

func (w *webhookMessage) Header(k, v string) contracts.WebhookMessage {
	if w.headers == nil {
		w.headers = make(map[string]string)
	}
	w.headers[k] = v
	return w
}

func (w *webhookMessage) JSON(v any) contracts.WebhookMessage {
	data, err := json.Marshal(v)
	if err != nil {
		w.jsonErr = fmt.Errorf("notify: Webhook 正文 JSON 序列化失败: %w", err)
		return w
	}
	w.body = data
	w.autoCT = true
	w.Header("Content-Type", "application/json")
	return w
}

func (w *webhookMessage) Body(b []byte) contracts.WebhookMessage {
	w.body = b
	if w.autoCT {
		delete(w.headers, "Content-Type")
		w.autoCT = false
	}
	return w
}

func (w *webhookMessage) IdempotencyKey(key string) contracts.WebhookMessage {
	w.idemKey = key
	return w
}

func (w *webhookMessage) Retry(times int) contracts.WebhookMessage {
	w.retry = &times
	return w
}

func (w *webhookMessage) RetryInterval(d time.Duration) contracts.WebhookMessage {
	w.retryInterval = &d
	return w
}

// key 返回幂等键：未设置时自动生成（框架 id 包），生成后固定复用。
func (w *webhookMessage) key() string {
	if w.idemKey == "" {
		w.idemKey = id.New()
	}
	return w.idemKey
}

func (w *webhookMessage) validate() error {
	if w.url == "" {
		return errors.New("notify: Webhook URL 不能为空")
	}
	return nil
}

// httpClient 返回发送用 HTTP client；构造时未注入则按配置惰性构造。
func (w *webhookMessage) httpClient() *http.Client {
	if w.client != nil {
		return w.client
	}
	w.client = &http.Client{Timeout: parseDurationCfg(w.cfg, cfgWebhookTimeout, 10*time.Second)}
	return w.client
}

// Send 同步发送（含内置幂等重试）。
func (w *webhookMessage) Send() error {
	if err := w.validate(); err != nil {
		return err
	}
	if w.jsonErr != nil {
		return w.jsonErr
	}

	key := w.key()
	retry := w.retryCount()
	interval := w.backoffInterval()

	var err error
	for attempt := 0; attempt <= retry; attempt++ {
		if attempt > 0 {
			time.Sleep(interval << (attempt - 1))
		}
		err = w.post(key)
		if err == nil {
			w.logResult(nil)
			return nil
		}
		if !isRetryable(err) {
			break
		}
	}
	w.logResult(err)
	return err
}

// Dispatch 异步发送：消息进入队列（不走内置重试，幂等键随 payload 传递）。
func (w *webhookMessage) Dispatch(opts ...contracts.DispatchOption) error {
	if err := w.validate(); err != nil {
		return err
	}
	if w.jsonErr != nil {
		return w.jsonErr
	}
	return dispatchMessage("webhook", webhookPayload{
		URL:            w.url,
		Headers:        w.headers,
		Body:           w.body,
		IdempotencyKey: w.key(),
	}, opts...)
}

// post 执行一次 POST 请求；返回的 error 实现了 isRetryable 判定。
func (w *webhookMessage) post(key string) error {
	req, err := http.NewRequest(http.MethodPost, w.url, bytes.NewReader(w.body))
	if err != nil {
		return &webhookError{err: err, retryable: false}
	}
	for k, v := range w.headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Idempotency-Key", key)

	resp, err := w.httpClient().Do(req)
	if err != nil {
		return &webhookError{err: err, retryable: true}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return &webhookError{
		err:       fmt.Errorf("notify: Webhook 返回 %s", resp.Status),
		retryable: retryableStatus(resp.StatusCode),
	}
}

// retryCount 同步发送内置重试次数：消息覆盖 > 配置 > 默认 3。
func (w *webhookMessage) retryCount() int {
	if w.retry != nil {
		return *w.retry
	}
	return w.cfg.GetInt(cfgWebhookRetry, 3)
}

// backoffInterval 退避基数：消息覆盖 > 配置 > 默认 1s。
func (w *webhookMessage) backoffInterval() time.Duration {
	if w.retryInterval != nil {
		return *w.retryInterval
	}
	return parseDurationCfg(w.cfg, cfgWebhookRetryInterval, time.Second)
}

// logResult 记录发送结果日志：成功 DEBUG，失败 WARN。
func (w *webhookMessage) logResult(err error) {
	if w.log == nil {
		return
	}
	if err != nil {
		w.log.WithError(err).WithField("channel", "webhook").
			Warn("[GoFast] notify: Webhook 发送失败")
		return
	}
	w.log.WithField("channel", "webhook").
		Debugf("[GoFast] notify: Webhook 已发送到 %s", w.url)
}

// webhookError 携带错误与是否可重试标记。
type webhookError struct {
	err       error
	retryable bool
}

func (e *webhookError) Error() string { return e.err.Error() }
func (e *webhookError) Unwrap() error { return e.err }

// isRetryable 判断错误是否可重试。
func isRetryable(err error) bool {
	var we *webhookError
	if errors.As(err, &we) {
		return we.retryable
	}
	return false
}

// retryableStatus 5xx 与 408/429 可重试，其余 4xx 不可重试。
func retryableStatus(code int) bool {
	if code >= 500 {
		return true
	}
	return code == http.StatusRequestTimeout || code == http.StatusTooManyRequests
}

// parseDurationCfg 读取 Duration 型字符串配置（如 "10s"），非法或缺省返回默认值。
func parseDurationCfg(cfg contracts.Config, key string, def time.Duration) time.Duration {
	s := cfg.GetString(key)
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return def
	}
	return d
}

// sendWebhookPayload 同步发送 Webhook payload（队列 worker 的执行路径）。
func sendWebhookPayload(p webhookPayload) error {
	w := newWebhookMessage(facades.Config(), facades.Log(), nil, p.URL)
	w.headers = p.Headers
	w.body = p.Body
	w.idemKey = p.IdempotencyKey
	return w.Send()
}

// decodeWebhookPayload 把任务参数还原为 webhookPayload（同 decodeMailPayload 的两态兼容）。
func decodeWebhookPayload(v any) (webhookPayload, error) {
	if p, ok := v.(webhookPayload); ok {
		return p, nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return webhookPayload{}, fmt.Errorf("notify: Webhook 载荷编码失败: %w", err)
	}
	var p webhookPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return webhookPayload{}, fmt.Errorf("notify: Webhook 载荷解码失败: %w", err)
	}
	return p, nil
}
