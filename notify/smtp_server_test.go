package notify

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zhoudm1743/go-fast-framework/mock"
)

// fakeSMTPServer 最小 SMTP 服务器：支持 none/starttls/tls 三种连接模式与 AUTH 声明。
type fakeSMTPServer struct {
	ln        net.Listener
	tlsMode   string // "" | "starttls" | "tls"
	authMechs string // 如 "PLAIN LOGIN"；空表示不声明 AUTH
	cert      tls.Certificate

	mu           sync.Mutex
	mailFrom     string
	rcptTo       []string
	data         []byte
	authenticated bool
	trace        []string
}

func startFakeSMTPServer(t *testing.T, tlsMode, authMechs string) *fakeSMTPServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeSMTPServer{ln: ln, tlsMode: tlsMode, authMechs: authMechs}
	if tlsMode != "" {
		s.cert = makeTestCert(t)
	}
	go s.serve()
	t.Cleanup(func() { _ = ln.Close() })
	return s
}

func (s *fakeSMTPServer) addr() string { return s.ln.Addr().String() }

func (s *fakeSMTPServer) host() string {
	h, _, _ := net.SplitHostPort(s.addr())
	return h
}

func (s *fakeSMTPServer) port() int {
	_, p, _ := net.SplitHostPort(s.addr())
	var n int
	_, _ = fmt.Sscanf(p, "%d", &n)
	return n
}

func (s *fakeSMTPServer) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *fakeSMTPServer) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	tlsActive := false
	write := func(s string) { _, _ = conn.Write([]byte(s)) }

	if s.tlsMode == "tls" {
		// 隐式 TLS：客户端连上即开始握手，greeting 必须在握手成功后发送
		conn = s.wrapTLS(conn)
		tlsActive = true
	}
	write("220 fake ESMTP ready\r\n")

	rd := bufio.NewReader(conn)

	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			return
		}
		s.mu.Lock()
		s.trace = append(s.trace, strings.TrimRight(line, "\r\n"))
		s.mu.Unlock()
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			write(s.ehloReply(tlsActive))
		case strings.HasPrefix(cmd, "STARTTLS"):
			write("220 Go ahead\r\n")
			conn = s.wrapTLS(conn)
			rd = bufio.NewReader(conn)
			tlsActive = true
		case strings.HasPrefix(cmd, "AUTH"):
			s.mu.Lock()
			s.authenticated = true
			s.mu.Unlock()
			write("235 Authentication accepted\r\n")
		case strings.HasPrefix(cmd, "MAIL"):
			s.mu.Lock()
			s.mailFrom = extractAddr(line)
			s.mu.Unlock()
			write("250 OK\r\n")
		case strings.HasPrefix(cmd, "RCPT"):
			s.mu.Lock()
			s.rcptTo = append(s.rcptTo, extractAddr(line))
			s.mu.Unlock()
			write("250 OK\r\n")
		case strings.HasPrefix(cmd, "DATA"):
			write("354 End data with <CR><LF>.<CR><LF>\r\n")
			data, _ := readSMTPData(rd)
			s.mu.Lock()
			s.data = data
			s.mu.Unlock()
			write("250 OK\r\n")
		case strings.HasPrefix(cmd, "QUIT"):
			write("221 Bye\r\n")
			return
		default:
			write("250 OK\r\n")
		}
	}
}

func (s *fakeSMTPServer) ehloReply(tlsActive bool) string {
	// net/smtp 的扩展解析会跳过第一行（视为服务器问候），扩展必须从第二行开始。
	lines := []string{"250-fake greets you"}
	if s.tlsMode == "starttls" && !tlsActive {
		lines = append(lines, "250-STARTTLS")
	}
	if s.authMechs != "" && (tlsActive || s.tlsMode == "") {
		lines = append(lines, "250-AUTH "+s.authMechs)
	}
	var b strings.Builder
	for i, l := range lines {
		if len(lines) == 1 || i == len(lines)-1 {
			b.WriteString(strings.Replace(l, "250-", "250 ", 1) + "\r\n")
		} else {
			b.WriteString(l + "\r\n")
		}
	}
	return b.String()
}

func (s *fakeSMTPServer) wrapTLS(conn net.Conn) net.Conn {
	tlsConn := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{s.cert}})
	_ = tlsConn.Handshake()
	return tlsConn
}

func (s *fakeSMTPServer) snapshot() (string, []string, []byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rcpt := make([]string, len(s.rcptTo))
	copy(rcpt, s.rcptTo)
	return s.mailFrom, rcpt, s.data, s.authenticated
}

func (s *fakeSMTPServer) commands() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.trace))
	copy(out, s.trace)
	return out
}

// extractAddr 从 "MAIL FROM:<a@b.com>" / "RCPT TO:<a@b.com>" 提取地址。
func extractAddr(line string) string {
	start := strings.Index(line, "<")
	end := strings.Index(line, ">")
	if start < 0 || end < 0 || end <= start {
		return strings.TrimSpace(line)
	}
	return line[start+1 : end]
}

// readSMTPData 读取 DATA 内容直到单独的 "." 行。
func readSMTPData(rd *bufio.Reader) ([]byte, error) {
	var b strings.Builder
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			return []byte(b.String()), err
		}
		if strings.TrimRight(line, "\r\n") == "." {
			return []byte(b.String()), nil
		}
		b.WriteString(line)
	}
}

// makeTestCert 生成自签名证书（含 127.0.0.1 IP SAN，匹配本机回环地址）。
func makeTestCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func smtpTestConfig(s *fakeSMTPServer, encryption string) *testConfig {
	return newTestConfig(map[string]any{
		"notify.mail.from":                  "noreply@example.com",
		"notify.mail.smtp.host":             s.host(),
		"notify.mail.smtp.port":             s.port(),
		"notify.mail.smtp.encryption":       encryption,
		"notify.mail.smtp.username":         "user",
		"notify.mail.smtp.password":         "pass",
		"notify.mail.smtp.tls_skip_verify":  true,
	})
}

func TestDeliverSMTP_Plain(t *testing.T) {
	s := startFakeSMTPServer(t, "", "")
	cfg := smtpTestConfig(s, "none")

	m := newMailMessage(cfg, mock.NewMockLog()).
		To("a@example.com").
		Subject("Hello").
		Text("plain body")
	if err := m.Send(); err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	from, rcpt, data, authed := s.snapshot()
	if from != "noreply@example.com" {
		t.Fatalf("MAIL FROM = %q", from)
	}
	if len(rcpt) != 1 || rcpt[0] != "a@example.com" {
		t.Fatalf("RCPT TO = %v", rcpt)
	}
	if authed {
		t.Fatal("server must not authenticate without AUTH extension")
	}
	raw := string(data)
	assertContains(t, raw, "Subject: Hello")
	assertContains(t, raw, "plain body")
	assertContains(t, raw, "From: noreply@example.com")
}

func TestDeliverSMTP_StartTLSWithLoginAuth(t *testing.T) {
	s := startFakeSMTPServer(t, "starttls", "LOGIN")
	cfg := smtpTestConfig(s, "starttls")

	m := newMailMessage(cfg, mock.NewMockLog()).
		To("a@example.com").
		Subject("s").
		Text("b")
	if err := m.Send(); err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	_, _, data, authed := s.snapshot()
	if !authed {
		t.Fatal("expected AUTH LOGIN over STARTTLS")
	}
	assertContains(t, string(data), "Subject: s")
}

func TestDeliverSMTP_ImplicitTLSWithPlainAuth(t *testing.T) {
	s := startFakeSMTPServer(t, "tls", "PLAIN")
	cfg := smtpTestConfig(s, "tls")

	m := newMailMessage(cfg, mock.NewMockLog()).
		To("a@example.com").
		Subject("s").
		Text("b")
	if err := m.Send(); err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	_, _, data, authed := s.snapshot()
	if !authed {
		t.Fatal("expected AUTH PLAIN over implicit TLS")
	}
	assertContains(t, string(data), "Subject: s")
}

func TestPickAuth_PriorityAndFallback(t *testing.T) {
	// CRAM-MD5 优先
	a := pickAuth("PLAIN LOGIN CRAM-MD5", "h", "u", "p")
	if a == nil {
		t.Fatal("CRAM-MD5 must be picked")
	}
	if _, ok := a.(*loginAuth); ok {
		t.Fatal("CRAM-MD5 must win over LOGIN")
	}
	// LOGIN 次之
	if _, ok := pickAuth("PLAIN LOGIN", "h", "u", "p").(*loginAuth); !ok {
		t.Fatal("LOGIN must be picked when CRAM-MD5 unavailable")
	}
	// PLAIN 兜底
	ap := pickAuth("PLAIN", "h", "u", "p")
	if ap == nil {
		t.Fatal("PLAIN must be the fallback")
	}
	if _, ok := ap.(*loginAuth); ok {
		t.Fatal("PLAIN fallback must not be LOGIN")
	}
	// 服务端不声明任何机制
	if pickAuth("", "h", "u", "p") != nil {
		t.Fatal("no auth must be selected when server declares nothing")
	}
}
