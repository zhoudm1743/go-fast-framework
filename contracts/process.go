package contracts

import (
	"io"
	"time"
)

// Process 进程管理服务契约，封装 os/exec 提供链式调用能力。
type Process interface {
	// Command 创建一个命令构造器。
	Command(name string, args ...string) ProcessBuilder
}

// ProcessBuilder 进程命令构造器，支持链式配置。
type ProcessBuilder interface {
	// Args 设置命令参数（覆盖创建时的参数）。
	Args(args ...string) ProcessBuilder
	// WorkingDir 设置工作目录。
	WorkingDir(dir string) ProcessBuilder
	// Env 设置单个环境变量；可多次调用。
	Env(key, value string) ProcessBuilder
	// Envs 批量设置环境变量。
	Envs(envs map[string]string) ProcessBuilder
	// Input 设置标准输入。
	Input(reader io.Reader) ProcessBuilder
	// InputString 通过字符串设置标准输入。
	InputString(input string) ProcessBuilder
	// Timeout 设置命令执行超时（0 表示不超时）。
	Timeout(timeout time.Duration) ProcessBuilder
	// Pipe 将另一个命令的 stdout 接到当前命令的 stdin，形成管道。
	Pipe(builder ProcessBuilder) ProcessBuilder

	// Run 同步执行并等待完成，返回执行结果。
	Run() (*ProcessResult, error)
	// Output 执行并只返回 stdout 字节。
	Output() ([]byte, error)
	// CombinedOutput 执行并合并 stdout/stderr 返回。
	CombinedOutput() ([]byte, error)
	// Start 异步启动进程，返回可进一步控制的结果对象。
	Start() (*ProcessResult, error)
	// Wait 等待已启动的进程结束（需先调用 Start）。
	Wait() (*ProcessResult, error)
	// Kill 强制终止进程。
	Kill() error
	// Signal 向进程发送信号。
	Signal(sig any) error
}

// ProcessResult 进程执行结果。
type ProcessResult struct {
	// Cmd 执行的命令行字符串（便于日志展示）。
	Cmd string
	// Stdout 标准输出。
	Stdout []byte
	// Stderr 标准错误。
	Stderr []byte
	// ExitCode 退出码；未退出时为 -1。
	ExitCode int
	// Success 是否成功退出（ExitCode == 0）。
	Success bool
	// Duration 执行耗时。
	Duration time.Duration
	// Error 执行过程中的错误（非退出码错误）。
	Error error
}
