package middleware

import (
	"strings"
	"testing"

	"github.com/zhoudm1743/go-fast-framework/auth"
	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/foundation"
	"github.com/zhoudm1743/go-fast-framework/jwt"
	"github.com/zhoudm1743/go-fast-framework/mock"
)

// mwCtx 中间件测试用 Context：实现 Header/Value/WithValue/Response/Next，其余 panic。
type mwCtx struct {
	contracts.Context
	headers    map[string]string
	values     map[string]any
	resp       *mwResponse
	method     string
	fullPath   string
	nextCalled bool
}

type mwResponse struct {
	contracts.Response
	unauthorized bool
	message      string
}

func (r *mwResponse) Unauthorized(msg ...string) error {
	r.unauthorized = true
	if len(msg) > 0 {
		r.message = msg[0]
	}
	return nil
}

func (c *mwCtx) Header(key string) string {
	// HTTP 头大小写不敏感
	for k, v := range c.headers {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}
func (c *mwCtx) Value(key string) any     { return c.values[key] }
func (c *mwCtx) WithValue(key string, value any) contracts.Context {
	c.values[key] = value
	return c
}
func (c *mwCtx) Response() contracts.Response { return c.resp }
func (c *mwCtx) Next() error                  { c.nextCalled = true; return nil }
func (c *mwCtx) Method() string               { return c.method }
func (c *mwCtx) FullPath() string             { return c.fullPath }

type mwUser struct{ id int64 }

func (u *mwUser) GetAuthID() any          { return u.id }
func (u *mwUser) GetAuthPassword() string { return "" }

func newMiddlewareTestApp(t *testing.T) foundation.Application {
	t.Helper()
	cfg := &mwTestConfig{m: map[string]any{
		"jwt.secret":         "mw-secret",
		"jwt.ttl":            60,
		"auth.default_guard": "api",
		"auth.guards.api":    map[string]any{"driver": "jwt", "provider": "users"},
	}}
	jwtSvc, err := jwt.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	app := foundation.NewApplication(".")
	app.Instance("config", cfg)
	app.Instance("jwt", jwtSvc)
	app.Instance("cache", mock.NewMockCache())
	app.Instance("auth", auth.NewManager(app))
	_ = app.MustMake("auth").(contracts.Auth).Provider("users", &mwProvider{user: &mwUser{id: 8}})
	return app
}

type mwProvider struct{ user contracts.AuthUser }

func (p *mwProvider) RetrieveByID(contracts.Context, any) (contracts.AuthUser, error) {
	return p.user, nil
}
func (p *mwProvider) RetrieveByIdentity(contracts.Context, string) (contracts.AuthUser, error) {
	return p.user, nil
}

func TestAuthMiddleware_InjectsUser(t *testing.T) {
	app := newMiddlewareTestApp(t)
	setFacadeApp(t, app)

	token, err := app.MustMake("auth").(contracts.Auth).Guard().LoginUsingID(nil, 8)
	if err != nil {
		t.Fatal(err)
	}

	ctx := &mwCtx{
		headers: map[string]string{"Authorization": "Bearer " + token},
		values:  map[string]any{},
		resp:    &mwResponse{},
	}
	if err := Auth()(ctx); err != nil {
		t.Fatal(err)
	}
	if !ctx.nextCalled {
		t.Fatal("next must be called on authenticated request")
	}
	if ctx.values[string(contracts.CtxKeyUserID)] != int64(8) {
		t.Fatalf("CtxKeyUserID = %v", ctx.values[string(contracts.CtxKeyUserID)])
	}
	if _, ok := ctx.values[string(contracts.CtxKeyAuthUser)].(*mwUser); !ok {
		t.Fatalf("CtxKeyAuthUser = %T", ctx.values[string(contracts.CtxKeyAuthUser)])
	}
}

func TestAuthMiddleware_RejectsUnauthenticated(t *testing.T) {
	app := newMiddlewareTestApp(t)
	setFacadeApp(t, app)

	ctx := &mwCtx{headers: map[string]string{}, values: map[string]any{}, resp: &mwResponse{}}
	if err := Auth()(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.nextCalled {
		t.Fatal("next must not be called")
	}
	if !ctx.resp.unauthorized {
		t.Fatal("expected 401 response")
	}
}

func TestAuthMiddleware_NamedGuard(t *testing.T) {
	app := newMiddlewareTestApp(t)
	setFacadeApp(t, app)

	token, _ := app.MustMake("auth").(contracts.Auth).Guard("api").LoginUsingID(nil, 8)
	ctx := &mwCtx{
		headers: map[string]string{"Authorization": token},
		values:  map[string]any{},
		resp:    &mwResponse{},
	}
	if err := Auth("api")(ctx); err != nil || !ctx.nextCalled {
		t.Fatalf("Auth(api) = %v, next=%v", err, ctx.nextCalled)
	}
}
