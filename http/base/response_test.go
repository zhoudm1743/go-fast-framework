package base

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// fakeContext 是仅服务于 Response 单元测试的假 contracts.Context（全接口实现）。
// 它把写出内容记录到内存 buffer，供断言“写出了什么、写了几次”；
// jsonErr 非 nil 时模拟底层 JSON 写失败，用于验证真实错误不被哨兵吞掉。
type fakeContext struct {
	buf       strings.Builder
	status    int
	headers   map[string]string
	sendFile  string
	jsonCalls int
	strCalls  int
	jsonErr   error
}

// 编译期保证实现了 contracts.Context 全接口。
var _ contracts.Context = (*fakeContext)(nil)

func (f *fakeContext) JSON(code int, obj any) error {
	f.jsonCalls++
	if f.jsonErr != nil {
		return f.jsonErr
	}
	f.status = code
	b, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	f.buf.Write(b)
	return nil
}

func (f *fakeContext) String(code int, s string) error {
	f.strCalls++
	f.status = code
	f.buf.WriteString(s)
	return nil
}

func (f *fakeContext) HTML(code int, name string, _ any) error {
	f.status = code
	f.buf.WriteString("html:" + name)
	return nil
}

func (f *fakeContext) SendFile(path string) error {
	f.sendFile = path
	return nil
}

func (f *fakeContext) Storage() contracts.Storage { return nil }

func (f *fakeContext) SetHeader(key, value string) contracts.Context {
	if f.headers == nil {
		f.headers = make(map[string]string)
	}
	f.headers[key] = value
	return f
}

func (f *fakeContext) Status(code int) contracts.Context { f.status = code; return f }

func (f *fakeContext) Response() contracts.Response { return NewResponse(f) }

func (f *fakeContext) Write(data []byte) error {
	f.buf.Write(data)
	return nil
}

func (f *fakeContext) WithValue(_ string, _ any) contracts.Context { return f }

// ── 以下方法与本测试无关，返回零值即可 ──────────────────────────────

func (f *fakeContext) Method() string                                    { return "" }
func (f *fakeContext) Path() string                                      { return "" }
func (f *fakeContext) Param(string) string                               { return "" }
func (f *fakeContext) Query(string, ...string) string                    { return "" }
func (f *fakeContext) QueryInt(string, ...int) int                       { return 0 }
func (f *fakeContext) QueryInt64(string, ...int64) int64                 { return 0 }
func (f *fakeContext) QueryFloat64(string, ...float64) float64           { return 0 }
func (f *fakeContext) QueryBool(string, ...bool) bool                    { return false }
func (f *fakeContext) Header(string) string                              { return "" }
func (f *fakeContext) IP() string                                        { return "" }
func (f *fakeContext) BodyRaw() []byte                                   { return nil }
func (f *fakeContext) FormValue(string) string                           { return "" }
func (f *fakeContext) ContentType() string                               { return "" }
func (f *fakeContext) UserAgent() string                                 { return "" }
func (f *fakeContext) FullPath() string                                  { return "" }
func (f *fakeContext) Bind(any) error                                    { return nil }
func (f *fakeContext) File(string) (contracts.File, error)               { return nil, nil }
func (f *fakeContext) Files(string) ([]contracts.File, error)            { return nil, nil }
func (f *fakeContext) Redirect(int, string) error                        { return nil }
func (f *fakeContext) Value(string) any                                  { return nil }
func (f *fakeContext) Next() error                                       { return nil }
func (f *fakeContext) Abort() error                                      { return nil }
func (f *fakeContext) AbortWithCode(int) error                           { return nil }
func (f *fakeContext) AbortWithJson(int, any) error                      { return nil }
func (f *fakeContext) Cookie(string) string                              { return "" }
func (f *fakeContext) SetCookie(string, string, contracts.CookieOptions) {}
func (f *fakeContext) ClearCookie(string)                                {}

// ── 哨兵语义测试 ────────────────────────────────────────────────────

// Success 成功写出后必须返回哨兵，且 JSON 内容恰好写出一次。
func TestResponseSuccessReturnsSentinelAndWritesOnce(t *testing.T) {
	fc := &fakeContext{}
	err := NewResponse(fc).Success(map[string]string{"item": "order"})

	if !contracts.IsResponseSent(err) {
		t.Fatalf("Success 成功写出后应返回 ErrResponseSent 哨兵，实际 %v", err)
	}
	if fc.jsonCalls != 1 {
		t.Fatalf("JSON 应恰好写出一次，实际 %d 次", fc.jsonCalls)
	}
	body := fc.buf.String()
	// 假 Context 只记录 JSON 的一次写出结果，body 与期望完全一致即证明“写出一次且内容正确”
	if want := `{"code":0,"message":"ok","data":{"item":"order"}}`; body != want {
		t.Fatalf("响应内容不正确: 期望 %s，实际 %s", want, body)
	}
	if fc.status != http.StatusOK {
		t.Fatalf("期望状态码 200，实际 %d", fc.status)
	}
}

// NotFound 同样返回哨兵，且状态码与业务码正确。
func TestResponseNotFoundReturnsSentinel(t *testing.T) {
	fc := &fakeContext{}
	err := NewResponse(fc).NotFound("订单不存在")

	if !contracts.IsResponseSent(err) {
		t.Fatalf("NotFound 成功写出后应返回哨兵，实际 %v", err)
	}
	if fc.jsonCalls != 1 || fc.status != http.StatusNotFound {
		t.Fatalf("期望写出一次 404，实际 calls=%d status=%d", fc.jsonCalls, fc.status)
	}
	body := fc.buf.String()
	// NotFound 走 Fail(404)，HTTP 状态码 404、业务码默认 0
	if want := `{"code":0,"message":"订单不存在"}`; body != want {
		t.Fatalf("404 响应内容不正确: 期望 %s，实际 %s", want, body)
	}
}

// Build 直接调用同样返回哨兵。
func TestResponseBuildReturnsSentinel(t *testing.T) {
	fc := &fakeContext{}
	err := NewResponse(fc).Build(http.StatusCreated, 0, "created", map[string]int{"id": 1})

	if !contracts.IsResponseSent(err) {
		t.Fatalf("Build 成功写出后应返回哨兵，实际 %v", err)
	}
	if fc.status != http.StatusCreated || fc.jsonCalls != 1 {
		t.Fatalf("期望写出一次 201，实际 status=%d calls=%d", fc.status, fc.jsonCalls)
	}
	if body := fc.buf.String(); !strings.Contains(body, `"id":1`) {
		t.Fatalf("响应内容不正确: %s", body)
	}
}

// 底层 JSON 写失败时必须返回真实错误（非哨兵），供调用方感知失败。
func TestResponseJSONFailureReturnsRealError(t *testing.T) {
	boom := errors.New("write failed")
	fc := &fakeContext{jsonErr: boom}
	err := NewResponse(fc).Build(http.StatusOK, 0, "ok", "x")

	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("写失败时应原样返回底层错误，实际 %v", err)
	}
	if contracts.IsResponseSent(err) {
		t.Fatal("写失败时不应返回哨兵，否则调用方误认为已响应")
	}
}

// String / Write / View 成功后同样返回哨兵。
func TestResponseStringWriteViewReturnSentinel(t *testing.T) {
	fc := &fakeContext{}
	resp := NewResponse(fc)

	if err := resp.String("hello"); !contracts.IsResponseSent(err) {
		t.Fatalf("String 成功后应返回哨兵，实际 %v", err)
	}
	if err := resp.Write([]byte("world")); !contracts.IsResponseSent(err) {
		t.Fatalf("Write 成功后应返回哨兵，实际 %v", err)
	}
	if err := resp.View("home/index.html", nil); !contracts.IsResponseSent(err) {
		t.Fatalf("View 成功后应返回哨兵，实际 %v", err)
	}
	if got := fc.buf.String(); got != "helloworldhtml:home/index.html" {
		t.Fatalf("写出内容不正确: %q", got)
	}
}

// File 在 storage 缺失时走 Fail 分支：内部 r.Fail 写出后返回的哨兵应自然向上传播。
func TestResponseFilePropagatesSentinelFromFail(t *testing.T) {
	fc := &fakeContext{} // Storage() 返回 nil
	err := NewResponse(fc).File("a.txt")

	if !contracts.IsResponseSent(err) {
		t.Fatalf("storage 缺失时 Fail 写出 500 后应传播哨兵，实际 %v", err)
	}
	if fc.status != http.StatusInternalServerError {
		t.Fatalf("期望 500，实际 %d", fc.status)
	}
}
