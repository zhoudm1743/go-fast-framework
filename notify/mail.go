package notify

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/id"
)

const (
	cfgMailFrom         = "notify.mail.from"
	cfgSMTPHost         = "notify.mail.smtp.host"
	cfgSMTPPort         = "notify.mail.smtp.port"
	cfgSMTPUsername     = "notify.mail.smtp.username"
	cfgSMTPPassword     = "notify.mail.smtp.password"
	cfgSMTPEncryption   = "notify.mail.smtp.encryption"
	cfgTLSSkipVerify    = "notify.mail.smtp.tls_skip_verify"
)

// mailMessage 实现 contracts.MailMessage。
type mailMessage struct {
	cfg     contracts.Config
	log     contracts.Log
	payload mailPayload
}

// newMailMessage 创建邮件消息构造器。
func newMailMessage(cfg contracts.Config, log contracts.Log) *mailMessage {
	return &mailMessage{cfg: cfg, log: log}
}

func (m *mailMessage) To(addrs ...string) contracts.MailMessage {
	m.payload.To = append(m.payload.To, addrs...)
	return m
}

func (m *mailMessage) Subject(s string) contracts.MailMessage {
	m.payload.Subject = s
	return m
}

func (m *mailMessage) Text(body string) contracts.MailMessage {
	m.payload.Text = body
	return m
}

func (m *mailMessage) HTML(body string) contracts.MailMessage {
	m.payload.HTML = body
	return m
}

func (m *mailMessage) From(addr string) contracts.MailMessage {
	m.payload.From = addr
	return m
}

func (m *mailMessage) validate() error {
	if len(m.payload.To) == 0 {
		return errors.New("notify: 收件人不能为空")
	}
	if m.payload.Subject == "" && m.payload.Text == "" && m.payload.HTML == "" {
		return errors.New("notify: 主题与正文不能同时为空")
	}
	return nil
}

// Send 同步发送。
func (m *mailMessage) Send() error {
	if err := m.validate(); err != nil {
		return err
	}
	p := m.payload
	if p.From == "" {
		p.From = m.cfg.GetString(cfgMailFrom)
	}
	if m.cfg.GetString(cfgSMTPHost) == "" {
		return errors.New("notify: 未配置 SMTP（notify.mail.smtp.host）")
	}

	raw := buildRFC822(p, "go-fast-"+id.New(), time.Now())
	err := deliverSMTP(m.cfg, p.From, p.To, raw)
	m.logResult(err)
	return err
}

// Dispatch 异步发送：消息进入队列，由队列 worker 执行。
func (m *mailMessage) Dispatch(opts ...contracts.DispatchOption) error {
	if err := m.validate(); err != nil {
		return err
	}
	return dispatchMessage("mail", m.payload, opts...)
}

// logResult 记录发送结果日志：成功 DEBUG，失败 WARN。
func (m *mailMessage) logResult(err error) {
	if m.log == nil {
		return
	}
	if err != nil {
		m.log.WithError(err).WithField("channel", "mail").
			Warn("[GoFast] notify: 邮件发送失败")
		return
	}
	m.log.WithField("channel", "mail").
		Debugf("[GoFast] notify: 邮件已发送给 %v", m.payload.To)
}

// ── RFC 822 组装 ────────────────────────────────────────────────────

// buildRFC822 组装完整邮件内容（头 + MIME 正文）。
// boundary 由调用方生成；now 注入以便测试。
func buildRFC822(p mailPayload, boundary string, now time.Time) []byte {
	var b strings.Builder
	b.WriteString("From: " + encodeHeaderValue(p.From) + "\r\n")
	b.WriteString("To: " + encodeHeaderValue(strings.Join(p.To, ", ")) + "\r\n")
	b.WriteString("Subject: " + encodeHeaderValue(p.Subject) + "\r\n")
	b.WriteString("Date: " + now.UTC().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")

	switch {
	case p.Text != "" && p.HTML != "":
		b.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary))
		b.WriteString("--" + boundary + "\r\n")
		writeMIMEPart(&b, `text/plain; charset="UTF-8"`, "quoted-printable", qpEncode(p.Text))
		b.WriteString("--" + boundary + "\r\n")
		writeMIMEPart(&b, `text/html; charset="UTF-8"`, "base64", b64Encode(p.HTML))
		b.WriteString("--" + boundary + "--\r\n")
	case p.HTML != "":
		b.WriteString(`Content-Type: text/html; charset="UTF-8"` + "\r\n")
		b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
		b.WriteString(b64Encode(p.HTML))
	default:
		b.WriteString(`Content-Type: text/plain; charset="UTF-8"` + "\r\n")
		b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		b.WriteString(qpEncode(p.Text))
	}
	return []byte(b.String())
}

// writeMIMEPart 写一个 MIME 子部件（不含 boundary 分隔行）。
func writeMIMEPart(b *strings.Builder, contentType, cte, body string) {
	b.WriteString("Content-Type: " + contentType + "\r\n")
	b.WriteString("Content-Transfer-Encoding: " + cte + "\r\n\r\n")
	b.WriteString(body)
	b.WriteString("\r\n")
}

// encodeHeaderValue 非 ASCII 头按 RFC 2047 encoded-word 编码（B 编码）。
func encodeHeaderValue(s string) string {
	if s == "" || isASCII(s) {
		return s
	}
	return mime.BEncoding.Encode("UTF-8", s)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// qpEncode quoted-printable 编码（行宽 76）。
func qpEncode(s string) string {
	var buf bytes.Buffer
	w := quotedprintable.NewWriter(&buf)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return buf.String()
}

// b64Encode base64 编码并按 76 字符折行（CRLF）。
func b64Encode(s string) string {
	enc := base64.StdEncoding.EncodeToString([]byte(s))
	if len(enc) <= 76 {
		return enc + "\r\n"
	}
	var b strings.Builder
	for len(enc) > 76 {
		b.WriteString(enc[:76])
		b.WriteString("\r\n")
		enc = enc[76:]
	}
	b.WriteString(enc)
	b.WriteString("\r\n")
	return b.String()
}

// ── SMTP 投递 ───────────────────────────────────────────────────────

// deliverSMTP 按配置建立 SMTP 连接并投递邮件。
func deliverSMTP(cfg contracts.Config, from string, to []string, raw []byte) error {
	host := cfg.GetString(cfgSMTPHost)
	port := cfg.GetInt(cfgSMTPPort, 25)
	username := cfg.GetString(cfgSMTPUsername)
	password := cfg.GetString(cfgSMTPPassword)
	encryption := cfg.GetString(cfgSMTPEncryption, "starttls")

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	tlsConf := &tls.Config{ServerName: host, InsecureSkipVerify: cfg.GetBool(cfgTLSSkipVerify)} //nolint:gosec // 显式配置项，用于自签/私有 CA
	var conn net.Conn
	var err error
	if encryption == "tls" {
		conn, err = tls.Dial("tcp", addr, tlsConf)
	} else {
		conn, err = net.DialTimeout("tcp", addr, 15*time.Second)
	}
	if err != nil {
		return fmt.Errorf("notify: 连接 SMTP 服务器失败: %w", err)
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("notify: 初始化 SMTP 客户端失败: %w", err)
	}
	defer client.Close()

	if encryption == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsConf); err != nil {
				return fmt.Errorf("notify: STARTTLS 升级失败: %w", err)
			}
		}
	}

	if username != "" {
		if ok, mechs := client.Extension("AUTH"); ok {
			if auth := pickAuth(mechs, host, username, password); auth != nil {
				if err := client.Auth(auth); err != nil {
					return fmt.Errorf("notify: SMTP 认证失败: %w", err)
				}
			}
		}
	}

	if err := client.Mail(from); err != nil {
		return fmt.Errorf("notify: MAIL FROM 失败: %w", err)
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("notify: RCPT TO %s 失败: %w", rcpt, err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("notify: DATA 命令失败: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		_ = w.Close()
		return fmt.Errorf("notify: 写入邮件内容失败: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("notify: 结束邮件内容失败: %w", err)
	}
	return client.Quit()
}

// pickAuth 按服务端支持的认证机制选择认证器（CRAM-MD5 > LOGIN > PLAIN）。
func pickAuth(mechs, host, username, password string) smtp.Auth {
	switch {
	case strings.Contains(mechs, "CRAM-MD5"):
		return smtp.CRAMMD5Auth(username, password)
	case strings.Contains(mechs, "LOGIN"):
		return &loginAuth{username: username, password: password}
	case strings.Contains(mechs, "PLAIN"):
		return smtp.PlainAuth("", username, password, host)
	}
	return nil
}

// loginAuth 实现 LOGIN 认证（net/smtp 内置缺失）。
type loginAuth struct {
	username string
	password string
	done     bool
}

func (a *loginAuth) Start(*smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", []byte(a.username), nil
}

func (a *loginAuth) Next(_ []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	if a.done {
		return nil, errors.New("notify: SMTP LOGIN 认证失败")
	}
	a.done = true
	return []byte(a.password), nil
}
