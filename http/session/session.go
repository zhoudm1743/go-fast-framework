package session

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// sessionData 存储单次会话的所有数据。
type sessionData struct {
	mu        sync.RWMutex
	id        string
	data      map[string]any
	flash     map[string]any
	readFlash map[string]struct{} // 本次请求已读取的 flash key，Save 时删除
	dirty     bool               // 数据是否有改动
	destroyed bool
}

func newSessionData(id string, existed map[string]any) *sessionData {
	data := make(map[string]any)
	if existed != nil {
		for k, v := range existed {
			data[k] = v
		}
	}
	flash := make(map[string]any)
	// 从 data 中恢复 flash 数据
	if raw, ok := data["__flash__"]; ok {
		if m, ok := raw.(map[string]any); ok {
			flash = m
		}
		delete(data, "__flash__")
	}
	return &sessionData{id: id, data: data, flash: flash, readFlash: make(map[string]struct{})}
}

func (s *sessionData) ID() string { return s.id }

func (s *sessionData) Get(key string) any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data[key]
}

func (s *sessionData) GetString(key string, def ...string) string {
	v := s.Get(key)
	if str, ok := v.(string); ok {
		return str
	}
	if len(def) > 0 {
		return def[0]
	}
	return ""
}

func (s *sessionData) GetInt(key string, def ...int) int {
	v := s.Get(key)
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	if len(def) > 0 {
		return def[0]
	}
	return 0
}

func (s *sessionData) GetBool(key string, def ...bool) bool {
	v := s.Get(key)
	if b, ok := v.(bool); ok {
		return b
	}
	if len(def) > 0 {
		return def[0]
	}
	return false
}

func (s *sessionData) Set(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
	s.dirty = true
}

func (s *sessionData) Has(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.data[key]
	return ok
}

func (s *sessionData) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	s.dirty = true
}

func (s *sessionData) Flash(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.flash == nil {
		s.flash = make(map[string]any)
	}
	s.flash[key] = value
	s.dirty = true
}

func (s *sessionData) GetFlash(key string) any {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.flash[key]
	s.readFlash[key] = struct{}{}
	return v
}

func (s *sessionData) Regenerate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.id = generateID()
	s.dirty = true
}

func (s *sessionData) Destroy() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = make(map[string]any)
	s.flash = make(map[string]any)
	s.destroyed = true
	s.dirty = true
}

// Save 由 MemoryStore 内部调用，把 flash 写回 data。
func (s *sessionData) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 清理已读取的 flash
	for k := range s.readFlash {
		delete(s.flash, k)
	}
	if len(s.flash) > 0 {
		s.data["__flash__"] = s.flash
	} else {
		delete(s.data, "__flash__")
	}
	return nil
}

// ── generateID ────────────────────────────────────────────────────────

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ── MemoryStore ───────────────────────────────────────────────────────

// memoryEntry 包含数据和过期时间。
type memoryEntry struct {
	data      map[string]any
	expiresAt time.Time
}

// MemoryStore 基于内存的会话存储，适合单实例开发环境。
// 不支持多实例/重启后恢复，生产环境请使用 Redis 驱动。
type MemoryStore struct {
	mu       sync.RWMutex
	entries  map[string]*memoryEntry
	lifetime time.Duration
	ticker   *time.Ticker
	done     chan struct{}
}

// NewMemoryStore 创建内存会话存储，lifetime 为会话有效期（0 = 不过期）。
func NewMemoryStore(lifetime time.Duration) *MemoryStore {
	s := &MemoryStore{
		entries:  make(map[string]*memoryEntry),
		lifetime: lifetime,
		done:     make(chan struct{}),
	}
	if lifetime > 0 {
		s.ticker = time.NewTicker(lifetime / 2)
		go s.gcLoop()
	}
	return s
}

func (s *MemoryStore) gcLoop() {
	for {
		select {
		case <-s.ticker.C:
			_ = s.GC()
		case <-s.done:
			return
		}
	}
}

func (s *MemoryStore) Stop() {
	if s.ticker != nil {
		s.ticker.Stop()
		close(s.done)
	}
}

func (s *MemoryStore) New(id string) (contracts.Session, error) {
	if id == "" {
		id = generateID()
		return newSessionData(id, nil), nil
	}
	s.mu.RLock()
	entry, ok := s.entries[id]
	s.mu.RUnlock()
	if !ok || (!entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt)) {
		// 过期或不存在，新建
		id = generateID()
		return newSessionData(id, nil), nil
	}
	return newSessionData(id, entry.data), nil
}

func (s *MemoryStore) save(sess *sessionData) error {
	if !sess.dirty {
		return nil
	}
	_ = sess.Save() // 处理 flash
	if sess.destroyed {
		s.mu.Lock()
		delete(s.entries, sess.id)
		s.mu.Unlock()
		return nil
	}
	data := make(map[string]any, len(sess.data))
	for k, v := range sess.data {
		data[k] = v
	}
	entry := &memoryEntry{data: data}
	if s.lifetime > 0 {
		entry.expiresAt = time.Now().Add(s.lifetime)
	}
	s.mu.Lock()
	s.entries[sess.id] = entry
	s.mu.Unlock()
	return nil
}

// SaveSession 将 Session 持久化（由 HTTP 中间件调用）。
func (s *MemoryStore) SaveSession(sess contracts.Session) error {
	if sd, ok := sess.(*sessionData); ok {
		return s.save(sd)
	}
	return nil
}

func (s *MemoryStore) Delete(id string) error {
	s.mu.Lock()
	delete(s.entries, id)
	s.mu.Unlock()
	return nil
}

func (s *MemoryStore) GC() error {
	now := time.Now()
	s.mu.Lock()
	for id, e := range s.entries {
		if !e.expiresAt.IsZero() && now.After(e.expiresAt) {
			delete(s.entries, id)
		}
	}
	s.mu.Unlock()
	return nil
}
