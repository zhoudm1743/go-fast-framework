package log

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// fakeConfig 实现 contracts.Config 用于测试。
type fakeConfig struct {
	values map[string]any
}

func newFakeConfig() *fakeConfig {
	return &fakeConfig{values: make(map[string]any)}
}

func (c *fakeConfig) set(key string, value any) *fakeConfig {
	c.values[key] = value
	return c
}

func (c *fakeConfig) GetString(key string, def ...string) string {
	d := ""
	if len(def) > 0 {
		d = def[0]
	}
	if v, ok := c.values[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return d
}

func (c *fakeConfig) GetInt(key string, def ...int) int {
	d := 0
	if len(def) > 0 {
		d = def[0]
	}
	if v, ok := c.values[key]; ok {
		if i, ok := v.(int); ok {
			return i
		}
	}
	return d
}

func (c *fakeConfig) GetBool(key string, def ...bool) bool {
	d := false
	if len(def) > 0 {
		d = def[0]
	}
	if v, ok := c.values[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return d
}

func (c *fakeConfig) GetFloat64(key string, def ...float64) float64 { return 0 }
func (c *fakeConfig) GetStringSlice(key string, def ...[]string) []string { return nil }
func (c *fakeConfig) GetStringMap(key string, def ...map[string]any) map[string]any {
	return nil
}
func (c *fakeConfig) Get(key string, def ...any) any {
	if len(def) > 0 {
		return def[0]
	}
	return nil
}
func (c *fakeConfig) Set(key string, value any)            {}
func (c *fakeConfig) SetDefaults(defaults map[string]any)         {}
func (c *fakeConfig) Add(namespace string, config map[string]any) {}
func (c *fakeConfig) Env(key string, defaultValue ...any) any {
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return nil
}

// =============================================================================
// parseLevel 测试
// =============================================================================

func TestParseLevel_Valid(t *testing.T) {
	tests := []struct {
		input string
		want  zapcore.Level
	}{
		{"debug", zapcore.DebugLevel},
		{"DEBUG", zapcore.DebugLevel},
		{"info", zapcore.InfoLevel},
		{"INFO", zapcore.InfoLevel},
		{"warn", zapcore.WarnLevel},
		{"error", zapcore.ErrorLevel},
		{"fatal", zapcore.FatalLevel},
		{"panic", zapcore.PanicLevel},
	}

	for _, tt := range tests {
		got, err := parseLevel(tt.input)
		if err != nil {
			t.Errorf("parseLevel(%q) 意外错误: %v", tt.input, err)
		}
		if got != tt.want {
			t.Errorf("parseLevel(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseLevel_Invalid(t *testing.T) {
	_, err := parseLevel("invalid")
	if err == nil {
		t.Error("parseLevel(\"invalid\") 应该返回错误")
	}
}

// =============================================================================
// NewLogger 模式测试
// =============================================================================

func TestNewLogger_ModeConsole(t *testing.T) {
	cfg := newFakeConfig().
		set("log.mode", "console").
		set("log.level", "debug").
		set("log.format", "text")

	log, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger(console) 失败: %v", err)
	}

	// 控制台模式不应该创建文件，Close 不应报错
	if err := log.(*logger).Close(); err != nil {
		t.Errorf("Close() 错误: %v", err)
	}
}

func TestNewLogger_ModeFile(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	cfg := newFakeConfig().
		set("log.mode", "file").
		set("log.level", "info").
		set("log.output_path", logPath)

	log, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger(file) 失败: %v", err)
	}

	log.Info("文件模式测试")

	// 需要 Close 刷新缓冲区
	if err := log.(*logger).Close(); err != nil {
		t.Errorf("Close() 错误: %v", err)
	}

	// 验证文件存在且包含日志
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("读取日志文件失败: %v", err)
	}
	if !strings.Contains(string(data), "文件模式测试") {
		t.Errorf("日志文件不包含预期内容: %s", string(data))
	}
}

func TestNewLogger_ModeFile_NoPath(t *testing.T) {
	cfg := newFakeConfig().
		set("log.mode", "file").
		set("log.level", "debug")

	_, err := NewLogger(cfg)
	if err == nil {
		t.Error("mode=file 但无 output_path 应该返回错误")
	}
}

func TestNewLogger_ModeHybrid(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	cfg := newFakeConfig().
		set("log.mode", "hybrid").
		set("log.level", "debug").
		set("log.output_path", logPath).
		set("log.format", "text")

	log, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger(hybrid) 失败: %v", err)
	}

	log.Info("混合模式测试")

	if err := log.(*logger).Close(); err != nil {
		t.Errorf("Close() 错误: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("读取日志文件失败: %v", err)
	}
	if !strings.Contains(string(data), "混合模式测试") {
		t.Errorf("日志文件不包含预期内容: %s", string(data))
	}
}

func TestNewLogger_InvalidLevel(t *testing.T) {
	cfg := newFakeConfig().
		set("log.mode", "console").
		set("log.level", "bad_level")

	_, err := NewLogger(cfg)
	if err == nil {
		t.Error("无效级别应该返回错误")
	}
}

func TestNewLogger_InvalidMode(t *testing.T) {
	cfg := newFakeConfig().
		set("log.mode", "bad_mode").
		set("log.level", "debug")

	_, err := NewLogger(cfg)
	if err == nil {
		t.Error("无效 mode 应该返回错误")
	}
}

// =============================================================================
// Release 模式级别钳制测试
// =============================================================================

func TestNewLogger_ReleaseMode(t *testing.T) {
	// release 模式 + debug：debug 被过滤，info 及以上通过
	buf := captureStdout(t, func() {
		cfg := newFakeConfig().
			set("log.mode", "console").
			set("log.level", "debug").
			set("log.format", "text").
			set("server.mode", "release")

		log, err := NewLogger(cfg)
		if err != nil {
			t.Fatalf("NewLogger(release) 失败: %v", err)
		}

		log.Debug("不应出现")
		log.Info("应该出现")
		log.Error("也应该出现")
	})

	if strings.Contains(buf, "不应出现") {
		t.Errorf("release 模式下 Debug 日志应该被过滤，但输出了: %s", buf)
	}
	if !strings.Contains(buf, "应该出现") {
		t.Errorf("release 模式下 Info 日志应该输出: %s", buf)
	}
	if !strings.Contains(buf, "也应该出现") {
		t.Errorf("release 模式下 Error 日志不应该被钳制: %s", buf)
	}
}

// =============================================================================
// 链式调用测试
// =============================================================================

func TestLogEntry_WithField(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	cfg := newFakeConfig().
		set("log.mode", "file").
		set("log.level", "debug").
		set("log.output_path", logPath).
		set("log.file_format", "json")

	log, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger 失败: %v", err)
	}

	log.WithField("user_id", 42).WithField("action", "login").Info("用户登录")

	if err := log.(*logger).Close(); err != nil {
		t.Errorf("Close() 错误: %v", err)
	}

	data, _ := os.ReadFile(logPath)
	content := string(data)
	if !strings.Contains(content, "user_id") || !strings.Contains(content, "42") {
		t.Errorf("链式调用字段未出现在输出中: %s", content)
	}
	if !strings.Contains(content, "action") || !strings.Contains(content, "login") {
		t.Errorf("链式调用字段未出现在输出中: %s", content)
	}
}

func TestLogEntry_WithFields(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	cfg := newFakeConfig().
		set("log.mode", "file").
		set("log.level", "debug").
		set("log.output_path", logPath).
		set("log.file_format", "json")

	log, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger 失败: %v", err)
	}

	log.WithFields(map[string]any{
		"module": "auth",
		"status": 200,
	}).Info("多个字段")

	if err := log.(*logger).Close(); err != nil {
		t.Errorf("Close() 错误: %v", err)
	}

	data, _ := os.ReadFile(logPath)
	content := string(data)
	if !strings.Contains(content, "module") || !strings.Contains(content, "auth") {
		t.Errorf("WithFields 字段未出现: %s", content)
	}
	if !strings.Contains(content, "status") {
		t.Errorf("WithFields 字段未出现: %s", content)
	}
}

func TestLogEntry_WithError(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	cfg := newFakeConfig().
		set("log.mode", "file").
		set("log.level", "debug").
		set("log.output_path", logPath).
		set("log.file_format", "json")

	log, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger 失败: %v", err)
	}

	testErr := errors.New("连接超时")
	log.WithError(testErr).Error("请求失败")

	if err := log.(*logger).Close(); err != nil {
		t.Errorf("Close() 错误: %v", err)
	}

	data, _ := os.ReadFile(logPath)
	content := string(data)
	if !strings.Contains(content, "error") || !strings.Contains(content, "连接超时") {
		t.Errorf("WithError 字段未出现: %s", content)
	}
}

func TestLogEntry_WithContext(t *testing.T) {
	cfg := newFakeConfig().
		set("log.mode", "console").
		set("log.level", "debug").
		set("log.format", "text")

	log, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger 失败: %v", err)
	}

	ctx := context.WithValue(context.Background(), "trace_id", "abc123")
	entry := log.WithContext(ctx)

	// WithContext 返回一个新的 logEntry，不应 panic
	if entry == nil {
		t.Error("WithContext 不应返回 nil")
	}
	entry.Info("context 测试")

	_ = ctx // 当前仅确保不 panic
}

// =============================================================================
// Printf（GORM Writer）测试
// =============================================================================

func TestLogger_Printf(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	cfg := newFakeConfig().
		set("log.mode", "file").
		set("log.level", "info").
		set("log.output_path", logPath).
		set("log.file_format", "json")

	log, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger 失败: %v", err)
	}

	// GORM 风格：已格式化消息，无参数
	log.(*logger).Printf("SELECT * FROM users WHERE id = 1\n")

	// GORM 风格：带参数
	log.(*logger).Printf("SELECT * FROM %s WHERE id = %d", "users", 1)

	if err := log.(*logger).Close(); err != nil {
		t.Errorf("Close() 错误: %v", err)
	}

	data, _ := os.ReadFile(logPath)
	content := string(data)
	if !strings.Contains(content, "SELECT * FROM users") {
		t.Errorf("Printf 输出未包含 SQL: %s", content)
	}
	// 确保没有 %!s(MISSING) 残留
	if strings.Contains(content, "%!s(MISSING)") {
		t.Errorf("Printf 不应包含 MISSING 占位符: %s", content)
	}
}

func readLastJSONLogLine(t *testing.T, logPath string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var m map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestLogger_CallerInfo_Printf(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	cfg := newFakeConfig().
		set("log.mode", "file").
		set("log.level", "debug").
		set("log.output_path", logPath).
		set("log.file_format", "json")

	log, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger 失败: %v", err)
	}

	log.(*logger).Printf("SELECT 1\n")

	if err := log.(*logger).Close(); err != nil {
		t.Errorf("Close() 错误: %v", err)
	}

	m := readLastJSONLogLine(t, logPath)
	callerFile, _ := m["caller_file"].(string)
	if strings.Contains(callerFile, "logger.go") {
		t.Errorf("Printf caller_file 不应指向 logger.go: %s", callerFile)
	}
	if !strings.Contains(callerFile, "logger_test.go") {
		t.Errorf("Printf caller_file 应指向测试文件: %s", callerFile)
	}
}

func TestLogger_CallerInfo_WithFields(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	cfg := newFakeConfig().
		set("log.mode", "file").
		set("log.level", "debug").
		set("log.output_path", logPath).
		set("log.file_format", "json")

	log, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger 失败: %v", err)
	}

	log.WithFields(map[string]any{"method": "GET"}).Info("http access")

	if err := log.(*logger).Close(); err != nil {
		t.Errorf("Close() 错误: %v", err)
	}

	m := readLastJSONLogLine(t, logPath)
	callerFile, _ := m["caller_file"].(string)
	if strings.Contains(callerFile, "logger.go") {
		t.Errorf("WithFields caller_file 不应指向 logger.go: %s", callerFile)
	}
	if !strings.Contains(callerFile, "logger_test.go") {
		t.Errorf("WithFields caller_file 应指向测试文件: %s", callerFile)
	}
}

// =============================================================================
// Caller 信息测试
// =============================================================================

func TestLogger_CallerInfo(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	cfg := newFakeConfig().
		set("log.mode", "file").
		set("log.level", "debug").
		set("log.output_path", logPath).
		set("log.file_format", "json")

	log, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger 失败: %v", err)
	}

	log.Info("caller 测试")

	if err := log.(*logger).Close(); err != nil {
		t.Errorf("Close() 错误: %v", err)
	}

	data, _ := os.ReadFile(logPath)
	content := string(data)
	if !strings.Contains(content, "logger_test.go") || !strings.Contains(content, "caller_file") {
		t.Errorf("日志应包含 caller 文件信息: %s", content)
	}
	if !strings.Contains(content, "caller_line") {
		t.Errorf("日志应包含 caller 行号: %s", content)
	}
}

// =============================================================================
// PrettyConsole Encoder 测试
// =============================================================================

func TestPrettyConsoleEncoder(t *testing.T) {
	cfg := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		MessageKey:     "msg",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05"),
		EncodeDuration: zapcore.SecondsDurationEncoder,
	}

	enc := newPrettyConsoleEncoder(cfg, false, "2006-01-02 15:04:05")

	entry := zapcore.Entry{
		Level:   zapcore.InfoLevel,
		Message: "测试消息",
		Time:    time.Now(),
	}

	fields := []zapcore.Field{
		zap.String("caller", "/home/user/project/framework/http/gin/route.go:42"),
		zap.String("caller_file", "/home/user/project/framework/http/gin/route.go"),
		zap.Int("caller_line", 42),
		zap.Int("status", 200),
		zap.String("method", "GET"),
	}

	buf, err := enc.EncodeEntry(entry, fields)
	if err != nil {
		t.Fatalf("EncodeEntry 失败: %v", err)
	}
	defer buf.Free()

	output := buf.String()
	if !strings.HasPrefix(output, "framework/http/gin/route.go:42\n") {
		t.Errorf("输出应以缩短后的 caller 行开头，实际: %q", output)
	}
	if !strings.Contains(output, "测试消息") {
		t.Errorf("输出应包含消息内容: %s", output)
	}
	if strings.Contains(output, "caller_file") || strings.Contains(output, "caller_line") {
		t.Errorf("输出不应包含冗余 caller 字段: %s", output)
	}
	if strings.Contains(output, "{") || strings.Contains(output, "}") {
		t.Errorf("控制台不应包含 JSON 格式: %s", output)
	}
	if !strings.Contains(output, "status=200") || !strings.Contains(output, "method=GET") {
		t.Errorf("输出应包含 key=value 字段: %s", output)
	}
}

func TestPrettyConsoleEncoder_NoCaller(t *testing.T) {
	cfg := zapcore.EncoderConfig{
		TimeKey:    "time",
		LevelKey:   "level",
		MessageKey: "msg",
		EncodeLevel: zapcore.LowercaseLevelEncoder,
		EncodeTime:  zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05"),
	}

	enc := newPrettyConsoleEncoder(cfg, false, "2006-01-02 15:04:05")

	entry := zapcore.Entry{
		Level:   zapcore.InfoLevel,
		Message: "无 caller 消息",
		Time:    time.Now(),
	}

	buf, err := enc.EncodeEntry(entry, nil)
	if err != nil {
		t.Fatalf("EncodeEntry 失败: %v", err)
	}
	defer buf.Free()

	output := buf.String()
	if !strings.Contains(output, "无 caller 消息") {
		t.Errorf("输出应包含消息内容: %s", output)
	}
	// 不应有前导 caller 行
	if strings.HasPrefix(output, "/") {
		t.Errorf("无 caller 时不应有空 caller 行: %s", output)
	}
}

// =============================================================================
// write / writef 测试（绕过 os.Exit/panic 以安全测试）
// =============================================================================

func TestLogger_Write(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	cfg := newFakeConfig().
		set("log.mode", "file").
		set("log.level", "debug").
		set("log.output_path", logPath).
		set("log.file_format", "json")

	log, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger 失败: %v", err)
	}

	l := log.(*logger)

	// 直接调用 write 方法（绕过 Fatal 的 os.Exit）
	l.write(zapcore.DebugLevel, "write 测试消息")
	l.writef(zapcore.InfoLevel, "writef 测试 %s", "消息")

	if err := log.(*logger).Close(); err != nil {
		t.Errorf("Close() 错误: %v", err)
	}

	data, _ := os.ReadFile(logPath)
	content := string(data)
	if !strings.Contains(content, "write 测试消息") {
		t.Errorf("write 未输出: %s", content)
	}
	if !strings.Contains(content, "writef 测试 消息") {
		t.Errorf("writef 未输出: %s", content)
	}
}

// =============================================================================
// 各级别测试
// =============================================================================

func TestLogger_Levels(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	cfg := newFakeConfig().
		set("log.mode", "file").
		set("log.level", "debug").
		set("log.output_path", logPath).
		set("log.file_format", "json")

	log, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger 失败: %v", err)
	}

	log.Debug("debug 消息")
	log.Debugf("debugf %s", "消息")
	log.Info("info 消息")
	log.Infof("infof %s", "消息")
	log.Warn("warn 消息")
	log.Warnf("warnf %s", "消息")
	log.Error("error 消息")
	log.Errorf("errorf %s", "消息")

	if err := log.(*logger).Close(); err != nil {
		t.Errorf("Close() 错误: %v", err)
	}

	data, _ := os.ReadFile(logPath)
	content := string(data)
	for _, keyword := range []string{
		"debug 消息", "debugf 消息",
		"info 消息", "infof 消息",
		"warn 消息", "warnf 消息",
		"error 消息", "errorf 消息",
	} {
		if !strings.Contains(content, keyword) {
			t.Errorf("日志未包含: %s", keyword)
		}
	}
}

// =============================================================================
// 级别过滤测试
// =============================================================================

func TestLogger_LevelFiltering(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	cfg := newFakeConfig().
		set("log.mode", "file").
		set("log.level", "error").
		set("log.output_path", logPath).
		set("log.file_format", "json")

	log, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger 失败: %v", err)
	}

	log.Debug("不应出现")
	log.Info("不应出现")
	log.Warn("不应出现")
	log.Error("应该出现")

	if err := log.(*logger).Close(); err != nil {
		t.Errorf("Close() 错误: %v", err)
	}

	data, _ := os.ReadFile(logPath)
	content := string(data)
	if strings.Contains(content, "不应出现") {
		t.Errorf("error 级别下不应该输出 Debug/Info/Warn 日志: %s", content)
	}
	if !strings.Contains(content, "应该出现") {
		t.Errorf("error 级别下应该输出 Error 日志: %s", content)
	}
}

// =============================================================================
// Close 幂等性测试
// =============================================================================

func TestLogger_CloseIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	cfg := newFakeConfig().
		set("log.mode", "file").
		set("log.level", "debug").
		set("log.output_path", logPath)

	log, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger 失败: %v", err)
	}

	l := log.(*logger)
	if err := l.Close(); err != nil {
		t.Errorf("第一次 Close() 错误: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Errorf("第二次 Close() 错误: %v", err)
	}
}

// =============================================================================
// ── v0.6.3 修复验证测试 ──────────────────────────────────────────────
// =============================================================================

// TestJSON_NoDuplicateCaller 验证 JSON 输出中不存在 duplicate caller 字段。
func TestJSON_NoDuplicateCaller(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	cfg := newFakeConfig().
		set("log.mode", "file").
		set("log.level", "debug").
		set("log.output_path", logPath).
		set("log.file_format", "json")

	log, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger 失败: %v", err)
	}

	log.Info("no dup caller")
	if err := log.(*logger).Close(); err != nil {
		t.Errorf("Close() 错误: %v", err)
	}

	data, _ := os.ReadFile(logPath)
	rawJSON := strings.TrimSpace(string(data))

	// 统计 "caller" 键出现次数（排除 caller_file/caller_line）
	callerKeyCount := 0
	idx := 0
	for {
		i := strings.Index(rawJSON[idx:], `"caller"`)
		if i < 0 {
			break
		}
		idx += i + len(`"caller"`)
		// 检查后面不是 _file 或 _line
		rest := rawJSON[idx:]
		if !strings.HasPrefix(rest, `_file`) && !strings.HasPrefix(rest, `_line`) {
			callerKeyCount++
		}
	}

	if callerKeyCount != 1 {
		t.Errorf(`期望 1 个 "caller" 键，实际 %d 个，存在重复`, callerKeyCount)
	}
}

// TestFieldMergeCore_NoAlias 验证多次 With() 不共享底层数组。
func TestFieldMergeCore_NoAlias(t *testing.T) {
	base := zapcore.NewNopCore()
	core := &fieldMergeCore{Core: base}

	c1 := core.With([]zapcore.Field{zap.String("a", "1")})
	c2 := core.With([]zapcore.Field{zap.String("b", "2")})

	fc1 := c1.(*fieldMergeCore)
	fc2 := c2.(*fieldMergeCore)

	if len(fc1.prefixFields) != 1 || fc1.prefixFields[0].Key != "a" {
		t.Errorf("core.With(a,1) 应只含 a=1，得到 %d 个字段", len(fc1.prefixFields))
	}
	if len(fc2.prefixFields) != 1 || fc2.prefixFields[0].Key != "b" {
		t.Errorf("core.With(b,2) 应只含 b=2，得到 %d 个字段", len(fc2.prefixFields))
	}

	// 链式调用：core.With(a).With(b)
	c3 := c1.With([]zapcore.Field{zap.String("c", "3")})
	fc3 := c3.(*fieldMergeCore)
	if len(fc3.prefixFields) != 2 {
		t.Fatalf("链式 With 应累积 2 个字段，得到 %d", len(fc3.prefixFields))
	}
	if fc3.prefixFields[0].Key != "a" || fc3.prefixFields[1].Key != "c" {
		t.Errorf("链式字段顺序错误: %v", fc3.prefixFields)
	}
}

// TestFormatFieldValue_AllTypes 验证 formatFieldValue 覆盖所有常用类型。
func TestFormatFieldValue_AllTypes(t *testing.T) {
	tests := []struct {
		name string
		field zapcore.Field
		want  string
	}{
		{"string", zap.String("k", "hello"), "hello"},
		{"str_with_space", zap.String("k", "hello world"), `"hello world"`},
		{"bool_true", zap.Bool("k", true), "true"},
		{"bool_false", zap.Bool("k", false), "false"},
		{"int", zap.Int("k", 42), "42"},
		{"int64", zap.Int64("k", 999), "999"},
		{"float64", zap.Float64("k", 3.14), "3.14"},
		{"duration", zap.Duration("k", time.Second), "1s"},
		{"time", zap.Time("k", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), "2026-01-01 00:00:00"},
		{"error", zap.Error(errors.New("boom")), "boom"},
		{"bytes", zap.ByteString("k", []byte("abc")), "abc"},
		{"stringer", zap.Stringer("k", &fakeStringer{"xyz"}), "xyz"},
		{"skip", zap.Skip(), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatFieldValue(tt.field)
			if got != tt.want {
				t.Errorf("formatFieldValue(%s) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

type fakeStringer struct{ s string }

func (f *fakeStringer) String() string { return f.s }

// TestDisplayCaller_Anchors 验证 displayCaller 对新增 anchor 的缩短。
func TestDisplayCaller_Anchors(t *testing.T) {
	tests := []struct {
		name   string
		caller string
		want   string
	}{
		{
			"http 包",
			"/home/user/go-fast-framework/http/gin/route.go:436",
			"http/gin/route.go:436",
		},
		{
			"log 包",
			"/home/user/go-fast-framework/log/logger.go:100",
			"log/logger.go:100",
		},
		{
			"facades 包",
			"/home/user/go-fast-framework/facades/cache.go:20",
			"facades/cache.go:20",
		},
		{
			"app 包（原有）",
			"/home/user/go-fast-framework/app/providers/log.go:50",
			"app/providers/log.go:50",
		},
		{
			"未知路径回退",
			"/home/user/go-fast-framework/foo/bar.go:10",
			"bar.go:10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := displayCaller(tt.caller)
			if got != tt.want {
				t.Errorf("displayCaller = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestShorten_EmptyFile 验证空文件名返回 unknown。
func TestShorten_EmptyFile(t *testing.T) {
	if s := shorten("", true); s != "unknown" {
		t.Errorf("空文件名应返回 unknown，得到 %q", s)
	}
	if s := shorten("", false); s != "unknown" {
		t.Errorf("ok=false 应返回 unknown，得到 %q", s)
	}
}

// TestPanic_UsesZapNative 验证 Panic 使用 zap 原生 panic（msg 来自日志内容）。
func TestPanic_UsesZapNative(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	cfg := newFakeConfig().
		set("log.mode", "file").
		set("log.level", "debug").
		set("log.output_path", logPath).
		set("log.file_format", "json")

	log, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger 失败: %v", err)
	}

	var panicked bool
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
				if s, ok := r.(string); ok {
					t.Logf("panic 消息: %q", s)
				}
			}
		}()
		log.Panic("should panic with this message")
	}()

	if !panicked {
		t.Fatal("Panic 应触发 panic")
	}

	if err := log.(*logger).Close(); err != nil {
		t.Errorf("Close() 错误: %v", err)
	}

	// 验证日志确实被写入（panic 前已记录）
	data, _ := os.ReadFile(logPath)
	if !strings.Contains(string(data), "should panic with this message") {
		t.Error("panic 前应写入日志")
	}
}

// =============================================================================
// 辅助函数
// =============================================================================

// captureStdout 捕获 os.Stdout 输出
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("创建 pipe 失败: %v", err)
	}
	os.Stdout = w

	// 写入 goroutine
	outC := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		outC <- buf.String()
	}()

	fn()

	w.Close()
	os.Stdout = old

	return <-outC
}
