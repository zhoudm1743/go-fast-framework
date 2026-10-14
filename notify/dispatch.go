package notify

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/facades"
)

// notifyJobSignature 队列任务签名（redis driver 按签名重建任务，必须固定）。
const notifyJobSignature = "go-fast:notify"

// notifyJob 队列任务：异步发送 notify 消息。必须无状态，
// redis driver 按签名通过注册表重建实例，运行期依赖从容器解析。
type notifyJob struct{}

func (notifyJob) Signature() string { return notifyJobSignature }

func (notifyJob) Handle(args ...any) error {
	if len(args) != 2 {
		return fmt.Errorf("notify: 任务参数数量错误: %d", len(args))
	}
	kind, ok := args[0].(string)
	if !ok {
		return errors.New("notify: 任务通道类型参数错误")
	}
	switch kind {
	case "mail":
		p, err := decodeMailPayload(args[1])
		if err != nil {
			return err
		}
		return sendMailPayload(p)
	case "webhook":
		p, err := decodeWebhookPayload(args[1])
		if err != nil {
			return err
		}
		return sendWebhookPayload(p)
	default:
		return fmt.Errorf("notify: 未知消息通道 %q", kind)
	}
}

// dispatchMessage 把消息 payload 打包为队列任务异步投递。
func dispatchMessage(kind string, payload any, opts ...contracts.DispatchOption) error {
	settings := contracts.DispatchSettings{}
	for _, o := range opts {
		o(&settings)
	}
	pending := facades.Queue().Job(notifyJob{}, []contracts.QueueArg{
		{Type: "string", Value: kind},
		{Type: "payload", Value: payload},
	})
	if settings.Queue != "" {
		pending = pending.OnQueue(settings.Queue)
	}
	if !settings.Delay.IsZero() {
		pending = pending.Delay(settings.Delay)
	}
	if settings.Retry > 0 {
		pending = pending.Retry(settings.Retry)
	}
	return pending.Dispatch()
}

// sendMailPayload 同步发送邮件 payload（队列 worker 的执行路径，与 Send 共用同一发送代码）。
func sendMailPayload(p mailPayload) error {
	m := newMailMessage(facades.Config(), facades.Log())
	m.payload = p
	return m.Send()
}

// decodeMailPayload 把任务参数还原为 mailPayload。
// 同步 driver 直达结构体；redis driver JSON 往返后是 map[string]any，需二次解码。
func decodeMailPayload(v any) (mailPayload, error) {
	if p, ok := v.(mailPayload); ok {
		return p, nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return mailPayload{}, fmt.Errorf("notify: 邮件载荷编码失败: %w", err)
	}
	var p mailPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return mailPayload{}, fmt.Errorf("notify: 邮件载荷解码失败: %w", err)
	}
	return p, nil
}
