package session

import (
	"net/http"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

const defaultCookieName = "go_fast_session"
const defaultLifetime = 2 * time.Hour

// Manager 实现 contracts.SessionManager。
type Manager struct {
	store      *MemoryStore
	cookieName string
	cookieOpts contracts.CookieOptions
	lifetime   time.Duration
}

// NewManager 创建会话管理器。
func NewManager(lifetime time.Duration, cookieName string) *Manager {
	if lifetime <= 0 {
		lifetime = defaultLifetime
	}
	if cookieName == "" {
		cookieName = defaultCookieName
	}
	return &Manager{
		store:      NewMemoryStore(lifetime),
		cookieName: cookieName,
		lifetime:   lifetime,
		cookieOpts: contracts.CookieOptions{
			MaxAge:   int(lifetime.Seconds()),
			Path:     "/",
			HTTPOnly: true,
			SameSite: "Lax",
		},
	}
}

func (m *Manager) Store(name ...string) contracts.SessionStore {
	return m.store
}

func (m *Manager) Session(ctx contracts.Context) (contracts.Session, error) {
	// 尝试从请求上下文中复用已加载的 Session
	if v := ctx.Value("__session__"); v != nil {
		if sess, ok := v.(contracts.Session); ok {
			return sess, nil
		}
	}
	// 从 Cookie 读取会话 ID
	id := ctx.Cookie(m.cookieName)
	sess, err := m.store.New(id)
	if err != nil {
		return nil, err
	}
	ctx.WithValue("__session__", sess)
	return sess, nil
}

func (m *Manager) SetCookieOptions(opts contracts.CookieOptions) {
	m.cookieOpts = opts
}

func (m *Manager) CookieName() string { return m.cookieName }

func (m *Manager) Lifetime() time.Duration { return m.lifetime }

// Middleware 返回一个 HandlerFunc，在请求处理完成后自动保存 Session 并写入 Cookie。
func (m *Manager) Middleware() contracts.HandlerFunc {
	return func(ctx contracts.Context) error {
		// 先执行后续 Handler
		if err := ctx.Next(); err != nil {
			return err
		}
		// 尝试从上下文中取 Session
		v := ctx.Value("__session__")
		if v == nil {
			return nil
		}
		sess, ok := v.(contracts.Session)
		if !ok {
			return nil
		}
		// 持久化
		if err := m.store.SaveSession(sess); err != nil {
			return err
		}

		// 如果会话被销毁，清除 Cookie
		opts := m.cookieOpts
		if sd, ok := sess.(*sessionData); ok && sd.destroyed {
			opts.MaxAge = -1
			ctx.SetCookie(m.cookieName, "", opts)
			return nil
		}
		// 写入或刷新 Cookie
		ctx.SetCookie(m.cookieName, sess.ID(), opts)
		return nil
	}
}

// SameSite 将字符串转换为 http.SameSite。
func SameSite(s string) http.SameSite {
	switch s {
	case "Strict":
		return http.SameSiteStrictMode
	case "None":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}
