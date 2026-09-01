package process

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// process 实现 contracts.Process。
type process struct{}

// NewProcess 创建 Process 服务实例。
func NewProcess() contracts.Process {
	return &process{}
}

func (p *process) Command(name string, args ...string) contracts.ProcessBuilder {
	return &processBuilder{
		name: name,
		args: args,
		env:  make(map[string]string),
	}
}

// processBuilder 实现 contracts.ProcessBuilder。
type processBuilder struct {
	name       string
	args       []string
	workingDir string
	env        map[string]string
	input      io.Reader
	timeout    time.Duration
	prev       *processBuilder
	piped      bool // 当前命令是否被后续命令管道连接

	cmd    *exec.Cmd
	cancel context.CancelFunc
	mu     sync.Mutex
	start  time.Time
	stdout bytes.Buffer
	stderr bytes.Buffer
}

func (b *processBuilder) Args(args ...string) contracts.ProcessBuilder {
	b.args = args
	return b
}

func (b *processBuilder) WorkingDir(dir string) contracts.ProcessBuilder {
	b.workingDir = dir
	return b
}

func (b *processBuilder) Env(key, value string) contracts.ProcessBuilder {
	b.env[key] = value
	return b
}

func (b *processBuilder) Envs(envs map[string]string) contracts.ProcessBuilder {
	for k, v := range envs {
		b.env[k] = v
	}
	return b
}

func (b *processBuilder) Input(reader io.Reader) contracts.ProcessBuilder {
	b.input = reader
	return b
}

func (b *processBuilder) InputString(input string) contracts.ProcessBuilder {
	b.input = strings.NewReader(input)
	return b
}

func (b *processBuilder) Timeout(timeout time.Duration) contracts.ProcessBuilder {
	b.timeout = timeout
	return b
}

func (b *processBuilder) Pipe(builder contracts.ProcessBuilder) contracts.ProcessBuilder {
	pb, ok := builder.(*processBuilder)
	if !ok {
		return b
	}
	pb.prev = b
	b.piped = true
	return pb
}

func (b *processBuilder) Run() (*contracts.ProcessResult, error) {
	result, err := b.Start()
	if err != nil {
		return result, err
	}
	return b.Wait()
}

func (b *processBuilder) Output() ([]byte, error) {
	res, err := b.Run()
	if res != nil {
		return res.Stdout, err
	}
	return nil, err
}

func (b *processBuilder) CombinedOutput() ([]byte, error) {
	res, err := b.Run()
	if res != nil {
		return append(res.Stdout, res.Stderr...), err
	}
	return nil, err
}

func (b *processBuilder) Start() (*contracts.ProcessResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.cmd != nil {
		return nil, fmt.Errorf("[GoFast] process: already started")
	}

	cmd, cancel, err := b.buildCmd()
	if err != nil {
		return nil, err
	}

	// 处理管道：先构建前驱命令，再通过 StdoutPipe 连接
	if b.prev != nil {
		prevCmd, prevCancel, err := b.prev.buildCmd()
		if err != nil {
			cancel()
			return nil, fmt.Errorf("[GoFast] process: build pipe source: %w", err)
		}
		stdout, err := prevCmd.StdoutPipe()
		if err != nil {
			cancel()
			prevCancel()
			return nil, fmt.Errorf("[GoFast] process: pipe stdout: %w", err)
		}
		cmd.Stdin = stdout
		b.prev.cmd = prevCmd
		b.prev.cancel = prevCancel
	}

	b.start = time.Now()
	b.cmd = cmd
	b.cancel = cancel

	// 先启动前驱，再启动当前命令
	if b.prev != nil {
		if err := b.prev.cmd.Start(); err != nil {
			return b.makeResult(-1, err), err
		}
	}

	if err := cmd.Start(); err != nil {
		return b.makeResult(-1, err), err
	}

	return b.makeResult(-1, nil), nil
}

func (b *processBuilder) Wait() (*contracts.ProcessResult, error) {
	b.mu.Lock()
	cmd := b.cmd
	b.mu.Unlock()

	if cmd == nil {
		return nil, fmt.Errorf("[GoFast] process: command not started")
	}

	err := cmd.Wait()
	if b.prev != nil {
		_, prevErr := b.prev.Wait()
		if err == nil {
			err = prevErr
		}
	}

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	return b.makeResult(exitCode, err), err
}

func (b *processBuilder) Kill() error {
	return b.kill()
}

func (b *processBuilder) kill() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.prev != nil {
		_ = b.prev.kill()
	}
	if b.cmd == nil || b.cmd.Process == nil {
		return nil
	}
	return b.cmd.Process.Kill()
}

func (b *processBuilder) Signal(sig any) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cmd == nil || b.cmd.Process == nil {
		return fmt.Errorf("[GoFast] process: command not started")
	}

	s, ok := sig.(syscall.Signal)
	if !ok {
		return fmt.Errorf("[GoFast] process: unsupported signal type %T", sig)
	}
	return b.cmd.Process.Signal(s)
}

func (b *processBuilder) buildCmd() (*exec.Cmd, context.CancelFunc, error) {
	var ctx context.Context
	var cancel context.CancelFunc
	if b.timeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), b.timeout)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}

	cmd := exec.CommandContext(ctx, b.name, b.args...)
	if b.workingDir != "" {
		cmd.Dir = b.workingDir
	}

	cmd.Env = os.Environ()
	for k, v := range b.env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	if b.input != nil {
		cmd.Stdin = b.input
	}

	// 若当前命令被后续命令管道连接，stdout 由 StdoutPipe 接管
	if !b.piped {
		cmd.Stdout = &b.stdout
	}
	cmd.Stderr = &b.stderr

	// 超时清理：监听 ctx，超时时直接杀掉进程（不依赖 b.mu）
	go func(c *exec.Cmd, done <-chan struct{}) {
		select {
		case <-done:
			return
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded && c.Process != nil {
				_ = c.Process.Kill()
			}
		}
	}(cmd, ctx.Done())

	return cmd, cancel, nil
}

func (b *processBuilder) makeResult(exitCode int, err error) *contracts.ProcessResult {
	duration := time.Duration(0)
	if !b.start.IsZero() {
		duration = time.Since(b.start)
	}

	cmdStr := b.name
	if len(b.args) > 0 {
		cmdStr += " " + strings.Join(b.args, " ")
	}

	return &contracts.ProcessResult{
		Cmd:      cmdStr,
		Stdout:   bytes.Clone(b.stdout.Bytes()),
		Stderr:   bytes.Clone(b.stderr.Bytes()),
		ExitCode: exitCode,
		Success:  exitCode == 0,
		Duration: duration,
		Error:    err,
	}
}
