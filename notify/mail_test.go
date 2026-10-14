package notify

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/zhoudm1743/go-fast-framework/mock"
)

var fixedNow = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

func TestBuildRFC822_TextOnly(t *testing.T) {
	p := mailPayload{
		From:    "noreply@example.com",
		To:      []string{"a@example.com"},
		Subject: "Hello",
		Text:    "plain body",
	}
	raw := string(buildRFC822(p, "BOUND", fixedNow))

	assertContains(t, raw, "From: noreply@example.com")
	assertContains(t, raw, "To: a@example.com")
	assertContains(t, raw, "Subject: Hello")
	assertContains(t, raw, "MIME-Version: 1.0")
	assertContains(t, raw, `Content-Type: text/plain; charset="UTF-8"`)
	assertContains(t, raw, "Content-Transfer-Encoding: quoted-printable")
	assertContains(t, raw, "plain body")
}

func TestBuildRFC822_HTMLBase64(t *testing.T) {
	p := mailPayload{
		From:    "noreply@example.com",
		To:      []string{"a@example.com"},
		Subject: "Hi",
		HTML:    "<p>html body</p>",
	}
	raw := string(buildRFC822(p, "BOUND", fixedNow))

	assertContains(t, raw, `Content-Type: text/html; charset="UTF-8"`)
	assertContains(t, raw, "Content-Transfer-Encoding: base64")
	assertContains(t, raw, base64.StdEncoding.EncodeToString([]byte("<p>html body</p>")))
}

func TestBuildRFC822_MultipartAlternative(t *testing.T) {
	p := mailPayload{
		From:    "noreply@example.com",
		To:      []string{"a@example.com"},
		Subject: "Both",
		Text:    "text part",
		HTML:    "<p>html part</p>",
	}
	raw := string(buildRFC822(p, "FIXEDBOUNDARY", fixedNow))

	assertContains(t, raw, `Content-Type: multipart/alternative; boundary="FIXEDBOUNDARY"`)
	assertContains(t, raw, "--FIXEDBOUNDARY")
	assertContains(t, raw, "text part")
	assertContains(t, raw, base64.StdEncoding.EncodeToString([]byte("<p>html part</p>")))
	assertContains(t, raw, "--FIXEDBOUNDARY--")
}

func TestBuildRFC822_NonASCIIHeaders(t *testing.T) {
	p := mailPayload{
		From:    "noreply@example.com",
		To:      []string{"a@example.com"},
		Subject: "订单通知",
	}
	raw := string(buildRFC822(p, "BOUND", fixedNow))

	// 非 ASCII 主题必须按 RFC 2047 编码，不允许以原始 UTF-8 出现在头里。
	if strings.Contains(raw, "Subject: 订单通知") {
		t.Fatal("subject must be RFC 2047 encoded, found raw UTF-8 in header")
	}
	if !strings.Contains(raw, "Subject: =?UTF-8?") {
		t.Fatalf("expected encoded-word subject, got header lines:\n%s", raw)
	}
}

func TestMailMessage_Validation(t *testing.T) {
	cfg := newTestConfig(map[string]any{})
	log := mock.NewMockLog()

	// 空收件人
	m := newMailMessage(cfg, log).Subject("s").Text("b")
	if err := m.Send(); err == nil || !strings.Contains(err.Error(), "收件人") {
		t.Fatalf("expected 收件人 validation error, got %v", err)
	}

	// 主题与正文全空
	m = newMailMessage(cfg, log).To("a@example.com")
	if err := m.Send(); err == nil {
		t.Fatal("expected error when subject and body are both empty")
	}

	// SMTP 未配置
	m = newMailMessage(cfg, log).To("a@example.com").Subject("s").Text("b")
	if err := m.Send(); err == nil || !strings.Contains(err.Error(), "SMTP") {
		t.Fatalf("expected SMTP config error, got %v", err)
	}
}

func TestMailMessage_BuilderPopulatesPayload(t *testing.T) {
	cfg := newTestConfig(map[string]any{})
	m := newMailMessage(cfg, mock.NewMockLog()).
		To("a@example.com", "b@example.com").
		Subject("s").
		Text("text").
		HTML("<p>html</p>").
		From("me@example.com")

	mm := m.(*mailMessage)
	if len(mm.payload.To) != 2 || mm.payload.To[1] != "b@example.com" {
		t.Fatalf("unexpected To: %v", mm.payload.To)
	}
	if mm.payload.Subject != "s" || mm.payload.Text != "text" ||
		mm.payload.HTML != "<p>html</p>" || mm.payload.From != "me@example.com" {
		t.Fatalf("unexpected payload: %+v", mm.payload)
	}
}

func assertContains(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Fatalf("expected to contain %q, got:\n%s", sub, s)
	}
}
