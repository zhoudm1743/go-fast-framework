package notify

import "github.com/zhoudm1743/go-fast-framework/contracts"

// logNotify 实现 contracts.LogNotify：把消息写入框架日志服务（开发调试用通道）。
type logNotify struct {
	log contracts.Log
}

// newLogNotify 创建日志通道。
func newLogNotify(log contracts.Log) *logNotify {
	return &logNotify{log: log}
}

func (l *logNotify) Debug(args ...any) { l.log.Debug(args...) }
func (l *logNotify) Info(args ...any)  { l.log.Info(args...) }
func (l *logNotify) Warn(args ...any)  { l.log.Warn(args...) }
func (l *logNotify) Error(args ...any) { l.log.Error(args...) }

func (l *logNotify) WithField(key string, value any) contracts.LogNotify {
	return newLogNotify(l.log.WithField(key, value))
}

func (l *logNotify) WithFields(fields map[string]any) contracts.LogNotify {
	return newLogNotify(l.log.WithFields(fields))
}

func (l *logNotify) WithError(err error) contracts.LogNotify {
	return newLogNotify(l.log.WithError(err))
}
