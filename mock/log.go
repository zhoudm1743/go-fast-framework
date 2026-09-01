package mock

import (
	"context"
	"sync"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// MockLog 实现 contracts.Log，用于测试。
type MockLog struct {
	mu sync.Mutex

	DebugFunc   func(args ...any)
	DebugfFunc  func(format string, args ...any)
	InfoFunc    func(args ...any)
	InfofFunc   func(format string, args ...any)
	WarnFunc    func(args ...any)
	WarnfFunc   func(format string, args ...any)
	ErrorFunc   func(args ...any)
	ErrorfFunc  func(format string, args ...any)
	FatalFunc   func(args ...any)
	FatalfFunc  func(format string, args ...any)
	PanicFunc   func(args ...any)
	PanicfFunc  func(format string, args ...any)
	WithFieldFunc  func(key string, value any) contracts.Log
	WithFieldsFunc func(fields map[string]any) contracts.Log
	WithErrorFunc  func(err error) contracts.Log
	WithContextFunc func(ctx context.Context) contracts.Log

	// Entries 记录所有日志调用。
	Entries []LogEntry
	// Fields 当前日志实例携带的字段。
	Fields map[string]any
}

// LogEntry 记录一次日志调用。
type LogEntry struct {
	Level   string
	Message string
	Args    []any
	Fields  map[string]any
}

// NewMockLog 创建 MockLog。
func NewMockLog() *MockLog {
	return &MockLog{
		Entries: make([]LogEntry, 0),
		Fields:  make(map[string]any),
	}
}

func (m *MockLog) record(level, msg string, args []any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Entries = append(m.Entries, LogEntry{
		Level:   level,
		Message: msg,
		Args:    args,
		Fields:  copyMap(m.Fields),
	})
}

func (m *MockLog) Debug(args ...any) {
	if m.DebugFunc != nil {
		m.DebugFunc(args...)
		return
	}
	m.record("debug", formatArgs(args), args)
}

func (m *MockLog) Debugf(format string, args ...any) {
	if m.DebugfFunc != nil {
		m.DebugfFunc(format, args...)
		return
	}
	m.record("debug", formatString(format, args), args)
}

func (m *MockLog) Info(args ...any) {
	if m.InfoFunc != nil {
		m.InfoFunc(args...)
		return
	}
	m.record("info", formatArgs(args), args)
}

func (m *MockLog) Infof(format string, args ...any) {
	if m.InfofFunc != nil {
		m.InfofFunc(format, args...)
		return
	}
	m.record("info", formatString(format, args), args)
}

func (m *MockLog) Warn(args ...any) {
	if m.WarnFunc != nil {
		m.WarnFunc(args...)
		return
	}
	m.record("warn", formatArgs(args), args)
}

func (m *MockLog) Warnf(format string, args ...any) {
	if m.WarnfFunc != nil {
		m.WarnfFunc(format, args...)
		return
	}
	m.record("warn", formatString(format, args), args)
}

func (m *MockLog) Error(args ...any) {
	if m.ErrorFunc != nil {
		m.ErrorFunc(args...)
		return
	}
	m.record("error", formatArgs(args), args)
}

func (m *MockLog) Errorf(format string, args ...any) {
	if m.ErrorfFunc != nil {
		m.ErrorfFunc(format, args...)
		return
	}
	m.record("error", formatString(format, args), args)
}

func (m *MockLog) Fatal(args ...any) {
	if m.FatalFunc != nil {
		m.FatalFunc(args...)
		return
	}
	m.record("fatal", formatArgs(args), args)
}

func (m *MockLog) Fatalf(format string, args ...any) {
	if m.FatalfFunc != nil {
		m.FatalfFunc(format, args...)
		return
	}
	m.record("fatal", formatString(format, args), args)
}

func (m *MockLog) Panic(args ...any) {
	if m.PanicFunc != nil {
		m.PanicFunc(args...)
		return
	}
	m.record("panic", formatArgs(args), args)
}

func (m *MockLog) Panicf(format string, args ...any) {
	if m.PanicfFunc != nil {
		m.PanicfFunc(format, args...)
		return
	}
	m.record("panic", formatString(format, args), args)
}

func (m *MockLog) WithField(key string, value any) contracts.Log {
	if m.WithFieldFunc != nil {
		return m.WithFieldFunc(key, value)
	}
	child := m.clone()
	child.Fields[key] = value
	return child
}

func (m *MockLog) WithFields(fields map[string]any) contracts.Log {
	if m.WithFieldsFunc != nil {
		return m.WithFieldsFunc(fields)
	}
	child := m.clone()
	for k, v := range fields {
		child.Fields[k] = v
	}
	return child
}

func (m *MockLog) WithError(err error) contracts.Log {
	if m.WithErrorFunc != nil {
		return m.WithErrorFunc(err)
	}
	return m.WithField("error", err)
}

func (m *MockLog) WithContext(ctx context.Context) contracts.Log {
	if m.WithContextFunc != nil {
		return m.WithContextFunc(ctx)
	}
	return m.WithField("context", ctx)
}

func (m *MockLog) clone() *MockLog {
	m.mu.Lock()
	defer m.mu.Unlock()
	return &MockLog{
		DebugFunc:       m.DebugFunc,
		DebugfFunc:      m.DebugfFunc,
		InfoFunc:        m.InfoFunc,
		InfofFunc:       m.InfofFunc,
		WarnFunc:        m.WarnFunc,
		WarnfFunc:       m.WarnfFunc,
		ErrorFunc:       m.ErrorFunc,
		ErrorfFunc:      m.ErrorfFunc,
		FatalFunc:       m.FatalFunc,
		FatalfFunc:      m.FatalfFunc,
		PanicFunc:       m.PanicFunc,
		PanicfFunc:      m.PanicfFunc,
		WithFieldFunc:   m.WithFieldFunc,
		WithFieldsFunc:  m.WithFieldsFunc,
		WithErrorFunc:   m.WithErrorFunc,
		WithContextFunc: m.WithContextFunc,
		Entries:         m.Entries,
		Fields:          copyMap(m.Fields),
	}
}

func formatArgs(args []any) string {
	if len(args) == 0 {
		return ""
	}
	if s, ok := args[0].(string); ok && len(args) == 1 {
		return s
	}
	return formatString("%v", args)
}

func formatString(format string, args []any) string {
	// 简单拼接，避免引入 fmt 格式化
	return format
}

func copyMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
