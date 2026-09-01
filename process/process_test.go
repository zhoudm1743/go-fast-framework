package process

import (
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestNewProcess(t *testing.T) {
	p := NewProcess()
	if p == nil {
		t.Fatal("process service should not be nil")
	}
}

func TestProcessOutput(t *testing.T) {
	p := NewProcess()
	out, err := p.Command("echo", "hello").Output()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(string(out)) != "hello" {
		t.Fatalf("expected 'hello', got %q", string(out))
	}
}

func TestProcessRun(t *testing.T) {
	p := NewProcess()
	res, err := p.Command("echo", "world").Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatal("expected success")
	}
	if res.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", res.ExitCode)
	}
	if strings.TrimSpace(string(res.Stdout)) != "world" {
		t.Fatalf("expected 'world', got %q", string(res.Stdout))
	}
}

func TestProcessEnv(t *testing.T) {
	p := NewProcess()
	out, err := p.Command("env").Env("GOFAST_TEST_KEY", "123").Output()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), "GOFAST_TEST_KEY=123") {
		t.Fatalf("expected env var in output, got %q", string(out))
	}
}

func TestProcessWorkingDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip on windows")
	}
	p := NewProcess()
	out, err := p.Command("pwd").WorkingDir("/tmp").Output()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), "/tmp") {
		t.Fatalf("expected /tmp in output, got %q", string(out))
	}
}

func TestProcessInputString(t *testing.T) {
	p := NewProcess()
	out, err := p.Command("cat").InputString("hello-input").Output()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != "hello-input" {
		t.Fatalf("expected 'hello-input', got %q", string(out))
	}
}

func TestProcessTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip on windows")
	}
	p := NewProcess()
	res, err := p.Command("sleep", "5").Timeout(100 * time.Millisecond).Run()
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if res == nil || res.Success {
		t.Fatal("expected failure result")
	}
}

func TestProcessPipe(t *testing.T) {
	p := NewProcess()
	out, err := p.Command("echo", "hello world").
		Pipe(p.Command("tr", " ", "-")).
		Output()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(string(out)) != "hello-world" {
		t.Fatalf("expected 'hello-world', got %q", string(out))
	}
}

func TestProcessSignal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip on windows")
	}
	p := NewProcess()
	pb := p.Command("sleep", "5")
	res, err := pb.Start()
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if res == nil {
		t.Fatal("expected result")
	}

	// 发送 SIGTERM
	err = pb.Signal(syscall.SIGTERM)
	if err != nil {
		t.Fatalf("signal failed: %v", err)
	}

	res, _ = pb.Wait()
	if res.ExitCode == 0 {
		t.Fatal("expected non-zero exit code after signal")
	}
}
