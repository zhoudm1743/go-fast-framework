package contracts

import "context"

// Log 日志服务契约。
type Log interface {
	Debug(args ...any)
	Debugf(format string, args ...any)
	Info(args ...any)
	Infof(format string, args ...any)
	Warn(args ...any)
	Warnf(format string, args ...any)
	Error(args ...any)
	Errorf(format string, args ...any)
	Fatal(args ...any)
	Fatalf(format string, args ...any)
	Panic(args ...any)
	Panicf(format string, args ...any)

	// WithField 添加单个字段，返回新的 Log 实例（链式调用）。
	WithField(key string, value any) Log
	// WithFields 添加多个字段，返回新的 Log 实例（链式调用）。
	WithFields(fields map[string]any) Log
	// WithError 添加 error 字段（等效于 WithField("error", err)）。
	WithError(err error) Log
	// WithContext 绑定 context（用于日志链路追踪等场景）。
	WithContext(ctx context.Context) Log
}
