package auth

import (
	"testing"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/jwt"
	"golang.org/x/crypto/bcrypt"
)

// newTestJWTGuard 构造 jwt guard：真实 jwt 服务 + stub provider + 内存 cache。
func newTestJWTGuard(t *testing.T, ttlMin int) (*jwtGuard, *stubProvider) {
	t.Helper()
	cfg := newTestConfig(map[string]any{
		"jwt.secret": "test-secret-key",
		"jwt.ttl":    ttlMin,
	})
	jwtSvc, err := jwt.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	provider := &stubProvider{}
	g := newJWTGuard("api", jwtSvc, provider, newStubCache())
	return g, provider
}

func TestJWTGuard_LoginParseRoundTrip(t *testing.T) {
	g, _ := newTestJWTGuard(t, 60)
	ctx := newStubCtx()

	token, err := g.Login(ctx, &testUser{ID: 42})
	if err != nil {
		t.Fatalf("Login() error: %v", err)
	}
	payload, err := g.Parse(ctx, token)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if payload.Guard != "api" {
		t.Fatalf("guard = %q", payload.Guard)
	}
	if payload.ID != float64(42) { // JSON 数值往返为 float64（记录在案的行为）
		t.Fatalf("id = %v (%T)", payload.ID, payload.ID)
	}
	if payload.ExpireAt.Before(time.Now()) || payload.IssuedAt.After(time.Now()) {
		t.Fatalf("bad times: %+v", payload)
	}
}

func TestJWTGuard_ParseBearerPrefix(t *testing.T) {
	g, _ := newTestJWTGuard(t, 60)
	ctx := newStubCtx()

	token, _ := g.LoginUsingID(ctx, 7)
	if _, err := g.Parse(ctx, "Bearer "+token); err != nil {
		t.Fatalf("Parse() with Bearer prefix error: %v", err)
	}
}

func TestJWTGuard_ExpiredToken(t *testing.T) {
	g, _ := newTestJWTGuard(t, -1) // ttl 负数分钟 → 签发即过期
	ctx := newStubCtx()

	token, err := g.LoginUsingID(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = g.Parse(ctx, token)
	assertErrIs(t, err, ErrTokenExpired)
}

func TestJWTGuard_LogoutRevokesToken(t *testing.T) {
	g, _ := newTestJWTGuard(t, 60)
	ctx := newStubCtx()

	token, _ := g.LoginUsingID(ctx, 1)
	ctx.headers["Authorization"] = "Bearer " + token

	if err := g.Logout(ctx); err != nil {
		t.Fatalf("Logout() error: %v", err)
	}
	_, err := g.Parse(ctx, token)
	assertErrIs(t, err, ErrTokenRevoked)
}

func TestJWTGuard_RefreshInvalidatesOldToken(t *testing.T) {
	g, _ := newTestJWTGuard(t, 60)
	ctx := newStubCtx()

	oldToken, _ := g.LoginUsingID(ctx, 1)
	ctx.headers["Authorization"] = "Bearer " + oldToken

	newToken, err := g.Refresh(ctx)
	if err != nil {
		t.Fatalf("Refresh() error: %v", err)
	}
	if newToken == oldToken {
		t.Fatal("refresh must return a new token")
	}
	if _, err := g.Parse(ctx, newToken); err != nil {
		t.Fatalf("new token must be valid: %v", err)
	}
	_, err = g.Parse(ctx, oldToken)
	assertErrIs(t, err, ErrTokenRevoked)
}

func TestJWTGuard_UserFromHeader(t *testing.T) {
	g, provider := newTestJWTGuard(t, 60)
	user := &testUser{ID: 9, Name: "alice"}
	provider.byIDFunc = func(ctx contracts.Context, id any) (contracts.AuthUser, error) {
		return user, nil
	}

	token, _ := g.LoginUsingID(newStubCtx(), 9)
	ctx := newStubCtx()
	ctx.headers["Authorization"] = token

	got, err := g.User(ctx)
	if err != nil {
		t.Fatalf("User() error: %v", err)
	}
	if got.(*testUser).Name != "alice" {
		t.Fatalf("user = %+v", got)
	}
	if len(provider.byIDCalls) != 1 || provider.byIDCalls[0] != float64(9) {
		t.Fatalf("provider calls = %v", provider.byIDCalls)
	}
}

func TestJWTGuard_UserFromInjectedCtx(t *testing.T) {
	g, provider := newTestJWTGuard(t, 60)
	user := &testUser{ID: 3}
	ctx := newStubCtx()
	ctx.WithValue(string(contracts.CtxKeyAuthUser), user)

	got, err := g.User(ctx)
	if err != nil || got != user {
		t.Fatalf("User() = %v, %v", got, err)
	}
	if len(provider.byIDCalls) != 0 {
		t.Fatal("provider must not be called when user is injected by middleware")
	}
}

func TestJWTGuard_Unauthenticated(t *testing.T) {
	g, _ := newTestJWTGuard(t, 60)
	ctx := newStubCtx()

	if _, err := g.User(ctx); true {
		assertErrIs(t, err, ErrUnauthenticated)
	}
	if g.Check(ctx) {
		t.Fatal("Check() must be false without token")
	}
	if _, err := g.ID(ctx); true {
		assertErrIs(t, err, ErrUnauthenticated)
	}
}

func TestJWTGuard_CheckAndID(t *testing.T) {
	g, _ := newTestJWTGuard(t, 60)
	token, _ := g.LoginUsingID(newStubCtx(), 11)
	ctx := newStubCtx()
	ctx.headers["Authorization"] = "Bearer " + token

	if !g.Check(ctx) {
		t.Fatal("Check() must be true with valid token")
	}
	id, err := g.ID(ctx)
	if err != nil || id != float64(11) {
		t.Fatalf("ID() = %v, %v", id, err)
	}
}

func TestJWTGuard_Attempt(t *testing.T) {
	g, provider := newTestJWTGuard(t, 60)
	hash, _ := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.DefaultCost)
	user := &testUser{ID: 5, Password: string(hash)}
	provider.byIdentityFunc = func(ctx contracts.Context, identity string) (contracts.AuthUser, error) {
		if identity == "alice" {
			return user, nil
		}
		return nil, ErrUserNotFound
	}

	ctx := newStubCtx()
	token, err := g.Attempt(ctx, "alice", "secret")
	if err != nil || token == "" {
		t.Fatalf("Attempt() = %q, %v", token, err)
	}

	// 密码错误与用户不存在统一返回 ErrInvalidCredentials
	if _, err := g.Attempt(ctx, "alice", "wrong"); true {
		assertErrIs(t, err, ErrInvalidCredentials)
	}
	if _, err := g.Attempt(ctx, "nobody", "secret"); true {
		assertErrIs(t, err, ErrInvalidCredentials)
	}
}
