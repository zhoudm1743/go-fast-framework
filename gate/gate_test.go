package gate

import (
	"testing"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

type mockContext struct {
	userID int
}

func (c *mockContext) Method() string                                 { return "" }
func (c *mockContext) Path() string                                   { return "" }
func (c *mockContext) Param(key string) string                        { return "" }
func (c *mockContext) Query(key string, defaultValue ...string) string { return "" }
func (c *mockContext) QueryInt(key string, defaultValue ...int) int     { return 0 }
func (c *mockContext) QueryInt64(key string, defaultValue ...int64) int64 { return 0 }
func (c *mockContext) QueryFloat64(key string, defaultValue ...float64) float64 { return 0 }
func (c *mockContext) QueryBool(key string, defaultValue ...bool) bool { return false }
func (c *mockContext) Header(key string) string                        { return "" }
func (c *mockContext) IP() string                                      { return "" }
func (c *mockContext) BodyRaw() []byte                                 { return nil }
func (c *mockContext) FormValue(key string) string                     { return "" }
func (c *mockContext) ContentType() string                             { return "" }
func (c *mockContext) UserAgent() string                               { return "" }
func (c *mockContext) FullPath() string                                { return "" }
func (c *mockContext) Bind(obj any) error                              { return nil }
func (c *mockContext) File(key string) (contracts.File, error)         { return nil, nil }
func (c *mockContext) Files(key string) ([]contracts.File, error)      { return nil, nil }
func (c *mockContext) Storage() contracts.Storage                      { return nil }
func (c *mockContext) SendFile(path string) error                      { return nil }
func (c *mockContext) JSON(code int, obj any) error                    { return nil }
func (c *mockContext) String(code int, s string) error                 { return nil }
func (c *mockContext) Redirect(code int, location string) error        { return nil }
func (c *mockContext) HTML(code int, name string, data any) error      { return nil }
func (c *mockContext) Response() contracts.Response                    { return nil }
func (c *mockContext) Status(code int) contracts.Context               { return c }
func (c *mockContext) SetHeader(key, value string) contracts.Context   { return c }
func (c *mockContext) Write(data []byte) error                         { return nil }
func (c *mockContext) Value(key string) any                            { return nil }
func (c *mockContext) WithValue(key string, value any) contracts.Context { return c }
func (c *mockContext) Next() error                                     { return nil }
func (c *mockContext) Abort() error                                    { return nil }
func (c *mockContext) AbortWithCode(code int) error                    { return nil }
func (c *mockContext) AbortWithJson(code int, obj any) error           { return nil }
func (c *mockContext) Cookie(name string) string                       { return "" }
func (c *mockContext) SetCookie(name, value string, opts contracts.CookieOptions) {}
func (c *mockContext) ClearCookie(name string)                           {}

var _ contracts.Context = (*mockContext)(nil)

type Post struct {
	ID     int
	UserID int
}

type PostPolicy struct{}

func (p *PostPolicy) Update(ctx contracts.Context, post *Post) bool {
	mc, ok := ctx.(*mockContext)
	return ok && mc.userID == post.UserID
}

func TestGateDefine(t *testing.T) {
	g := NewGate()
	g.Define("admin", func(ctx contracts.Context, args ...any) bool {
		mc, ok := ctx.(*mockContext)
		return ok && mc.userID == 1
	})

	if !g.Allows(&mockContext{userID: 1}, "admin") {
		t.Fatal("user 1 should be admin")
	}
	if g.Allows(&mockContext{userID: 2}, "admin") {
		t.Fatal("user 2 should not be admin")
	}
}

func TestGatePolicy(t *testing.T) {
	g := NewGate()
	g.Policy(&Post{}, &PostPolicy{})

	post := &Post{ID: 1, UserID: 1}
	if !g.Allows(&mockContext{userID: 1}, "update", post) {
		t.Fatal("owner should be allowed to update")
	}
	if g.Allows(&mockContext{userID: 2}, "update", post) {
		t.Fatal("non-owner should be denied")
	}
}

func TestGateBefore(t *testing.T) {
	g := NewGate()
	g.Define("edit", func(ctx contracts.Context, args ...any) bool { return true })
	g.Before(func(ctx contracts.Context, ability string, args ...any) contracts.GateResponse {
		mc, ok := ctx.(*mockContext)
		if ok && mc.userID == 0 {
			return newResponse(false, "banned")
		}
		return nil
	})

	if g.Allows(&mockContext{userID: 0}, "edit") {
		t.Fatal("banned user should be denied by before callback")
	}
	if !g.Allows(&mockContext{userID: 1}, "edit") {
		t.Fatal("normal user should be allowed")
	}
}

func TestGateUndefinedAbility(t *testing.T) {
	g := NewGate()
	res := g.Inspect(&mockContext{}, "missing")
	if res.Allowed() {
		t.Fatal("undefined ability should be denied")
	}
}

func TestAbilityToMethodName(t *testing.T) {
	cases := []struct {
		ability  string
		expected string
	}{
		{"update", "Update"},
		{"update-post", "UpdatePost"},
		{"view-any", "ViewAny"},
	}
	for _, c := range cases {
		if got := abilityToMethodName(c.ability); got != c.expected {
			t.Fatalf("abilityToMethodName(%q) = %q, want %q", c.ability, got, c.expected)
		}
	}
}
