package auth

import (
	"testing"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/foundation"
	"github.com/zhoudm1743/go-fast-framework/jwt"
)

// newAuthTestApp 装配带 jwt/cache/auth 的应用。
func newAuthTestApp(t *testing.T, cfgMap map[string]any) foundation.Application {
	t.Helper()
	if _, ok := cfgMap["jwt.secret"]; !ok {
		cfgMap["jwt.secret"] = "default-secret"
	}
	if _, ok := cfgMap["jwt.ttl"]; !ok {
		cfgMap["jwt.ttl"] = 60
	}
	cfg := newTestConfig(cfgMap)

	jwtSvc, err := jwt.New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	app := foundation.NewApplication(".")
	app.Instance("config", cfg)
	app.Instance("jwt", jwtSvc)
	app.Instance("cache", newStubCache())
	app.Instance("auth", NewManager(app))
	return app
}

func usersProvider() contracts.UserProvider { return &stubProvider{} }

func TestManager_DefaultGuard(t *testing.T) {
	app := newAuthTestApp(t, map[string]any{
		"auth.default_guard": "api",
		"auth.guards.api":    map[string]any{"driver": "jwt", "provider": "users"},
	})
	m := app.MustMake("auth").(contracts.Auth)
	_ = m.Provider("users", usersProvider())

	token, err := m.Guard().LoginUsingID(newStubCtx(), 1)
	if err != nil {
		t.Fatalf("LoginUsingID() error: %v", err)
	}
	payload, err := m.Guard().Parse(newStubCtx(), token)
	if err != nil || payload.Guard != "api" {
		t.Fatalf("payload = %+v, err = %v", payload, err)
	}
}

func TestManager_NamedGuardIsolation(t *testing.T) {
	app := newAuthTestApp(t, map[string]any{
		"auth.default_guard":  "api",
		"jwt.guards.admin":    map[string]any{"secret": "admin-secret", "ttl": 60},
		"auth.guards.api":     map[string]any{"driver": "jwt", "provider": "users"},
		"auth.guards.admin":   map[string]any{"driver": "jwt", "provider": "admins"},
	})
	m := app.MustMake("auth").(contracts.Auth)
	_ = m.Provider("users", usersProvider())
	_ = m.Provider("admins", usersProvider())

	ctx := newStubCtx()
	adminToken, err := m.Guard("admin").LoginUsingID(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	// admin 守卫使用 jwt.guards.admin 的密钥签发
	payload, err := m.Guard("admin").Parse(ctx, adminToken)
	if err != nil || payload.Guard != "admin" {
		t.Fatalf("payload = %+v, err = %v", payload, err)
	}
	// 默认密钥签发的 token 不能被 admin 守卫解析
	apiToken, _ := m.Guard("api").LoginUsingID(ctx, 1)
	if _, err := m.Guard("admin").Parse(ctx, apiToken); err == nil {
		t.Fatal("api token must not parse under admin guard")
	}
}

func TestManager_UnknownGuardPanics(t *testing.T) {
	app := newAuthTestApp(t, map[string]any{})
	m := app.MustMake("auth").(contracts.Auth)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for undeclared guard")
		}
	}()
	m.Guard("ghost")
}

func TestManager_ExtendAndProviderDuplicate(t *testing.T) {
	app := newAuthTestApp(t, map[string]any{})
	m := app.MustMake("auth").(contracts.Auth)

	if err := m.Provider("users", usersProvider()); err != nil {
		t.Fatal(err)
	}
	if err := m.Provider("users", usersProvider()); err == nil {
		t.Fatal("duplicate provider must error")
	}

	factory := func(name string, cfg map[string]any, p contracts.UserProvider) (contracts.Guard, error) {
		return nil, nil
	}
	if err := m.Extend("custom", factory); err != nil {
		t.Fatal(err)
	}
	if err := m.Extend("custom", factory); err == nil {
		t.Fatal("duplicate driver must error")
	}
	// 内置 jwt 驱动不允许覆盖
	if err := m.Extend("jwt", factory); err == nil {
		t.Fatal("built-in jwt driver must not be overridable")
	}
}

func TestManager_CustomGuardDriver(t *testing.T) {
	app := newAuthTestApp(t, map[string]any{
		"auth.default_guard":  "static",
		"auth.guards.static":  map[string]any{"driver": "static", "provider": "users"},
	})
	m := app.MustMake("auth").(contracts.Auth)
	_ = m.Provider("users", usersProvider())

	called := false
	_ = m.Extend("static", func(name string, cfg map[string]any, p contracts.UserProvider) (contracts.Guard, error) {
		called = true
		if name != "static" {
			t.Fatalf("guard name = %q", name)
		}
		return newJWTGuard(name, app.MustMake("jwt").(contracts.JWT), p, nil), nil
	})

	if _, err := m.Guard().LoginUsingID(newStubCtx(), 1); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("custom driver factory not called")
	}
}
