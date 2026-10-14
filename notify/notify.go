package notify

import (
	"net/http"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/foundation"
)

// notify 实现 contracts.Notify 门面。
type notify struct {
	app        foundation.Application
	httpClient *http.Client
}

// New 创建 notify 门面实例（注册到容器的 "notify" 服务）。
func New(app foundation.Application) contracts.Notify {
	return &notify{
		app: app,
		httpClient: &http.Client{
			Timeout: parseDurationCfg(app.Config(), cfgWebhookTimeout, 10*time.Second),
		},
	}
}

func (n *notify) Mail() contracts.MailMessage {
	return newMailMessage(n.app.Config(), n.app.Log())
}

func (n *notify) Webhook(url string) contracts.WebhookMessage {
	return newWebhookMessage(n.app.Config(), n.app.Log(), n.httpClient, url)
}

func (n *notify) Log() contracts.LogNotify {
	return newLogNotify(n.app.Log())
}
