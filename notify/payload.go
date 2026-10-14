package notify

// mailPayload 邮件消息的纯数据载荷（JSON 可序列化，用于队列异步投递）。
type mailPayload struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
	HTML    string   `json:"html"`
}

// webhookPayload Webhook 消息的纯数据载荷（JSON 可序列化，用于队列异步投递）。
type webhookPayload struct {
	URL            string            `json:"url"`
	Headers        map[string]string `json:"headers"`
	Body           []byte            `json:"body"` // JSON 中自动 base64
	IdempotencyKey string            `json:"idempotency_key"`
}
