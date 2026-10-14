# notify 消息提供者设计

日期：2026-10-14
状态：待评审
模块：`go-fast-framework`

## 1. 背景与目标

框架已有 cache / queue / event / schedule 等业务级能力，但缺少统一的外部消息发送通道。业务系统里"发验证码邮件、异常告警 webhook、订单通知"是刚需，目前用户只能裸调 `net/smtp` 或 `http.Post`，没有统一的重试、日志、异步投递。

目标：新增 `notify` 服务，提供统一门面，支持邮件（SMTP）、Webhook、日志三个通道，并复用框架现有 queue 支持异步发送。

## 2. 关键决策

| 决策点 | 结论 |
|---|---|
| 语义模型 | **B 路线：统一门面，调用方自选通道**（非 Laravel 式通知中心，不做多通道路由） |
| 结构 | **方案 A：薄门面**。无 channel 注册表，无 Manager；门面直接暴露消息构造器，driver 藏在实现里 |
| 异步 | v1 即接入现有 queue：`Dispatch()` 异步投递 |
| Webhook 重试 | 幂等重试：默认幂等键用框架 `id` 包生成（`id.New()`），**不用 UUID**；重试复用同一幂等键 |
| SMTP 依赖 | 标准库 `net/smtp` + 手写 MIME，**零新依赖** |
| 明确不做 | Cc/Bcc、附件、模板引擎、SMS/Slack 通道（接口预留，后续可加，不破现有接口） |

## 3. 目录结构

```
go-fast-framework/
  contracts/notify.go          # Notify / MailMessage / WebhookMessage / LogNotify / DispatchOption
  notify/
    service_provider.go        # 注册 "notify" 单例，Boot 挂 OnShutdown
    notify.go                  # 门面实现（implements contracts.Notify）
    mail.go                    # MailMessage builder + MIME 组装 + SMTP 发送
    webhook.go                 # WebhookMessage builder + HTTP POST + 幂等重试
    log.go                     # LogNotify，包装 contracts.Log
    payload.go                 # mailPayload / webhookPayload / notifyJob（QueueJob 实现）
    notify_test.go             # 门面与 builder 单测
    mail_test.go               # MIME golden 测试 + fake SMTP server 测试
    webhook_test.go            # httptest.Server 测试（含幂等重试断言）
    dispatch_test.go           # mock queue 测试 Dispatch 语义与 payload JSON 往返
  facades/notify.go            # facades.Notify()，与 facades.Cache() 同款
```

## 4. 契约 API（contracts/notify.go）

```go
package contracts

import "time"

// Notify 消息通知服务契约（facades.Notify() 返回此接口）。
// 统一的发送门面：调用方构造消息、自选通道；同步 Send 或异步 Dispatch。
type Notify interface {
    Mail() MailMessage
    Webhook(url string) WebhookMessage
    Log() LogNotify
}

// MailMessage 邮件消息构造器。
type MailMessage interface {
    To(addrs ...string) MailMessage
    Subject(s string) MailMessage
    Text(body string) MailMessage      // 纯文本正文
    HTML(body string) MailMessage      // HTML 正文；与 Text 同设时发 multipart/alternative
    From(addr string) MailMessage      // 覆盖配置中的默认发件人
    Send() error                       // 同步发送
    Dispatch(opts ...DispatchOption) error // 异步：进 queue
}

// WebhookMessage Webhook 消息构造器。
type WebhookMessage interface {
    Header(k, v string) WebhookMessage
    JSON(v any) WebhookMessage     // 序列化为 JSON body（自动设 Content-Type: application/json）
    Body(b []byte) WebhookMessage  // 原始 body；Content-Type 用 Header 另指定

    // IdempotencyKey 设置幂等键（Idempotency-Key 请求头）。
    // 不设置则发送时自动生成（框架 id 包，id.New()）。所有重试复用同一键。
    IdempotencyKey(key string) WebhookMessage

    // Retry 同步 Send 的内置重试次数（默认取配置 notify.webhook.retry）。
    // 仅影响同步路径；Dispatch 的重试由 WithRetry 交给 queue，见 §5。
    Retry(times int) WebhookMessage
    RetryInterval(d time.Duration) WebhookMessage // 退避基数，指数增长（默认取配置）

    Send() error
    Dispatch(opts ...DispatchOption) error
}

// LogNotify 日志通道：消息写入框架日志，供开发调试。
type LogNotify interface {
    Debug(args ...any)
    Info(args ...any)
    Warn(args ...any)
    Error(args ...any)
    WithField(key string, value any) LogNotify
    WithFields(fields map[string]any) LogNotify
    WithError(err error) LogNotify
}

// DispatchSettings Dispatch 选项集合。
type DispatchSettings struct {
    Queue string    // 队列名，空为默认
    Delay time.Time // 延迟到指定时间后处理
    Retry int       // queue 侧最大重试次数（不含首次）
}

// DispatchOption Dispatch 配置函数。
type DispatchOption func(*DispatchSettings)

func WithQueue(name string) DispatchOption
func WithDelay(t time.Time) DispatchOption
func WithRetry(times int) DispatchOption
```

## 5. 异步语义（关键）

- `Dispatch()` 把消息转成**纯数据 payload**（导出字段、JSON 可序列化——redis driver 硬约束），包成内部 `notifyJob`（实现 `contracts.QueueJob`）交给 `facades.Queue().Job(...)`。
- payload 定义（notify/payload.go）：

```go
type mailPayload struct {
    From    string   `json:"from"`
    To      []string `json:"to"`
    Subject string   `json:"subject"`
    Text    string   `json:"text"`
    HTML    string   `json:"html"`
}

type webhookPayload struct {
    URL            string            `json:"url"`
    Headers        map[string]string `json:"headers"`
    Body           []byte            `json:"body"`             // JSON 中自动 base64
    IdempotencyKey string            `json:"idempotency_key"`
}
```

- `notifyJob.Signature()` 固定返回 `"go-fast:notify"`；`Handle(args ...any)` 从 args 取通道类型与 payload，**从容器重新解析 config 与 sender** 后同步发送。payload 经 redis JSON 往返后是 `map[string]any`，需二次 decode（marshal 回 JSON 再 unmarshal 到结构体）；同步 driver 直达结构体——Handle 需兼容两种形态。
- 同步 `Send()` 与异步执行共用同一条发送代码路径，行为只差"在哪执行"。
- 队列重试 / 延迟语义完全复用现有 queue：`WithDelay` 映射 `QueuePending.Delay`，`WithRetry` 映射 `QueuePending.Retry`，`WithQueue` 映射 `OnQueue`。
- **Webhook 的 Dispatch 不走发送器内联重试**（worker 内 sleep 会占住 worker）；幂等键随 payload 进队列，重试交给 `WithRetry`，每次重试带同一个 `Idempotency-Key`，接收方天然安全。

## 6. SMTP 实现（notify/mail.go）

- 标准库 `net/smtp`，零新依赖。
- 连接策略按配置 `notify.mail.smtp.encryption`：
  - `tls`：implicit TLS（465），`tls.Dial` 后直接作为 SMTP 连接
  - `starttls`：明文连接，服务端支持时自动 STARTTLS 升级
  - `none`：明文，仅用于本地/测试环境
- 认证：PLAIN / CRAM-MD5 用 `net/smtp` 内置；LOGIN 手写 `smtp.Auth` 实现（`net/smtp` 内置没有，国内邮件服务商常见）。认证机制按服务端 EHLO 声明自动协商，优先级 CRAM-MD5 > LOGIN > PLAIN。
- MIME 手写：
  - 单 Text：text/plain；单 HTML：text/html；两者同设：multipart/alternative
  - 非 ASCII 头（主题、收件人显示名）按 RFC 2047 encoded-word 编码
  - body 用 quoted-printable（Text）/ base64（HTML）编码
- 发件人优先级：`From()` 覆盖 > 配置 `notify.mail.from`；收件人、主题、正文至少一项非空，否则 `Send()` 返回 error。

## 7. Webhook 实现（notify/webhook.go）

- `net/http` + 包级共享 client（timeout 取配置 `notify.webhook.timeout`）。
- 自动 `Idempotency-Key` 头：调用方未设置时，首次发送前用框架 `id` 包生成（`id.New()`），**同一条消息的所有重试复用同一键**。
- 同步 `Send()` 内置重试：
  - 可重试错误：网络错误、5xx、408、429
  - 不重试：其余 4xx（400/401/403/404…），立即返回 error
  - 次数默认配置 `notify.webhook.retry`（3），退避指数增长、基数 `notify.webhook.retry_interval`（1s）；均可用 `Retry()` / `RetryInterval()` 按消息覆盖
- JSON(v any) 序列化失败在 Send 时返回 error；`Body` 与 `JSON` 后设者覆盖先设者（互斥，只允许一个 body 来源）。

## 8. 配置

```yaml
notify:
  mail:
    from: no-reply@example.com
    smtp:
      host: smtp.example.com
      port: 465
      username: xxx
      password: xxx
      encryption: tls        # tls | starttls | none
      tls_skip_verify: false # true 时跳过证书校验（自签/私有 CA SMTP，默认 false）
  webhook:
    timeout: 10s
    retry: 3                 # 同步 Send 内置重试次数
    retry_interval: 1s       # 退避基数
```

读取方式与现有服务一致（`config` 契约，viper key）。`notify.mail.smtp.host` 为空时 `Mail().Send()` 返回"未配置 SMTP"错误（`Dispatch()` 前置校验同样拦截）。

## 9. 错误处理

- 参数错误（空收件人、空 URL、未配置 SMTP、JSON 序列化失败）：`Send()` 直接返回 error；`Dispatch()` 在入队前做同样校验，尽早暴露。
- 同步路径错误即返回给调用方；异步路径最终失败由 queue 记日志（现有行为），notify 不另建死信机制。
- 所有发送路径写框架 `log` 服务并带 `channel=mail|webhook` 字段：成功 DEBUG 级，失败 WARN/ERROR 级。

## 10. 测试方案（全部无外部网络依赖）

- **MIME 组装**：golden 测试（固定输入对比编码输出；multipart/alternative、RFC 2047、QP/base64）
- **SMTP 发送**：测试内起本地 fake SMTP server（`net.Listen` + 最小 SMTP 协议实现），覆盖 implicit TLS / STARTTLS / 三种认证
- **Webhook**：`httptest.Server` 覆盖成功、4xx 不重试、5xx 重试至成功、指数退避间隔、**多次重试 Idempotency-Key 头一致**
- **幂等键**：未设置时自动生成且符合框架 id 格式；用户设置后透传
- **Dispatch**：mock `contracts.Queue` 断言入队参数与 payload JSON 往返（含 `[]byte` body 的 base64 往返）；mock queue 失败路径
- **notifyJob.Handle**：同步 driver（结构体直达）与 redis 形态（map 二次 decode）两种入参
- **Log 通道**：mock `contracts.Log` 断言级别与字段
- 全量测试跑 `go test ./notify/...`，不依赖外部网络与外部服务

## 11. 验收标准

1. `go build ./...` 与 `go test ./...` 在 go-fast-framework 模块内全绿
2. `facades.Notify()` 可用，三个通道的 Send/Dispatch 行为符合本设计
3. 同步发送路径与队列异步路径产出一致（同一 payload 同一发送代码）
4. Webhook 重试全程幂等键不变
5. 零新增第三方依赖
