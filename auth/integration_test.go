package auth

import (
	"testing"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/facades"
	"github.com/zhoudm1743/go-fast-framework/foundation"
	"github.com/zhoudm1743/go-fast-framework/jwt"
)

// TestAuthIntegration_BootAndUse 端到端：provider 注册 → Boot → facades.Auth() 签发/解析。
func TestAuthIntegration_BootAndUse(t *testing.T) {
	cfg := newTestConfig(map[string]any{
		"jwt.secret":      "integration-secret",
		"jwt.ttl":         60,
		"auth.guards.api": map[string]any{"driver": "jwt", "provider": "users"},
	})
	jwtSvc, err := jwt.New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	app := foundation.NewApplication(".")
	app.Instance("config", cfg)
	app.Instance("jwt", jwtSvc)
	app.Instance("cache", newStubCache())
	app.SetProviders([]foundation.ServiceProvider{&ServiceProvider{}})
	app.Boot()

	// 业务方在自己的 ServiceProvider.Boot 中注册用户提供者
	if err := facades.Auth().Provider("users", usersProvider()); err != nil {
		t.Fatal(err)
	}

	ctx := newStubCtx()
	token, err := facades.Auth().Guard().LoginUsingID(ctx, 1)
	if err != nil {
		t.Fatalf("LoginUsingID() error: %v", err)
	}
	payload, err := facades.Auth().Guard().Parse(ctx, token)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if payload.Guard != "api" || payload.ID != float64(1) {
		t.Fatalf("payload = %+v", payload)
	}
}

var _ contracts.Auth = (*manager)(nil)
