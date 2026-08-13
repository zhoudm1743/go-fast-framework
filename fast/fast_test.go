package fast

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

type testCommand struct {
	sig    string
	desc   string
	execFn func(ctx contracts.ConsoleContext) error
}

func (c *testCommand) Signature() string   { return c.sig }
func (c *testCommand) Description() string { return c.desc }
func (c *testCommand) Extend() contracts.CommandExtend {
	return contracts.CommandExtend{}
}

func (c *testCommand) Handle(ctx contracts.ConsoleContext) error {
	if c.execFn != nil {
		return c.execFn(ctx)
	}
	return nil
}

func TestNewKernel(t *testing.T) {
	k := newKernel()
	if k == nil {
		t.Fatal("newKernel() 返回 nil")
	}
	if len(k.commands) < 8 {
		t.Errorf("应至少有 8 个内置命令, 实际 %d", len(k.commands))
	}
}

func TestRegister(t *testing.T) {
	k := newKernel()
	cmd := &testCommand{sig: "test:hello", desc: "测试命令"}
	k.Register([]contracts.ConsoleCommand{cmd})
	if _, ok := k.commands["test:hello"]; !ok {
		t.Error("Register 后应能找到注册的命令")
	}
}

func TestRegisterOverwrite(t *testing.T) {
	k := newKernel()
	cmd1 := &testCommand{sig: "mycmd", desc: "第一个"}
	cmd2 := &testCommand{sig: "mycmd", desc: "第二个"}
	k.Register([]contracts.ConsoleCommand{cmd1, cmd2})
	found := k.commands["mycmd"]
	if found.Description() != "第二个" {
		t.Error("同名命令应被覆盖为后注册的")
	}
}

func TestCallEmpty(t *testing.T) {
	k := newKernel()
	err := k.Call("")
	if err == nil {
		t.Error("空命令应返回错误")
	}
}

func TestCallSyncList(t *testing.T) {
	k := newKernel()
	err := k.CallSync("list")
	if err != nil {
		t.Errorf("list 命令应可执行: %v", err)
	}
}

func TestCallSyncHelp(t *testing.T) {
	k := newKernel()
	err := k.CallSync("help list")
	if err != nil {
		t.Errorf("help list 应可执行: %v", err)
	}
}

func TestCallSyncUnknownCommand(t *testing.T) {
	k := newKernel()
	err := k.CallSync("nonexistent")
	if err == nil {
		t.Error("不存在的命令应返回错误")
	}
}

func TestCallSyncRegisteredCommand(t *testing.T) {
	var executed bool
	k := newKernel()
	k.Register([]contracts.ConsoleCommand{
		&testCommand{
			sig:  "custom:test",
			desc: "自定义测试命令",
			execFn: func(ctx contracts.ConsoleContext) error {
				executed = true
				return nil
			},
		},
	})

	err := k.CallSync("custom:test")
	if err != nil {
		t.Errorf("自定义命令应可执行: %v", err)
	}
	if !executed {
		t.Error("自定义命令的 Handle 未被调用")
	}
}

func TestCallSyncWithArgs(t *testing.T) {
	var args []string
	k := newKernel()
	k.Register([]contracts.ConsoleCommand{
		&testCommand{
			sig: "args:test",
			execFn: func(ctx contracts.ConsoleContext) error {
				args = ctx.Arguments()
				return nil
			},
		},
	})

	err := k.CallSync("args:test arg1 --flag value")
	if err != nil {
		t.Errorf("带参数的命令应可执行: %v", err)
	}
	if len(args) == 0 {
		t.Error("Arguments() 应返回参数列表")
	}
}

func TestCallSyncWithOption(t *testing.T) {
	var optVal string
	k := newKernel()
	k.Register([]contracts.ConsoleCommand{
		&testCommand{
			sig: "opt:test",
			execFn: func(ctx contracts.ConsoleContext) error {
				optVal = ctx.Option("name")
				return nil
			},
		},
	})

	err := k.CallSync("opt:test --name value123")
	if err != nil {
		t.Errorf("带选项的命令应可执行: %v", err)
	}
	if optVal != "value123" {
		t.Errorf("Option(name) = %q, want %q", optVal, "value123")
	}
}

func TestRunSyncNilArgs(t *testing.T) {
	k := newKernel()
	err := k.RunSync(nil)
	if err != nil {
		t.Errorf("nil 参数应显示命令列表: %v", err)
	}
	err = k.RunSync([]string{})
	if err != nil {
		t.Errorf("空参数应显示命令列表: %v", err)
	}
}

func TestRunSyncHelpFlag(t *testing.T) {
	k := newKernel()
	err := k.RunSync([]string{"list", "--help"})
	if err != nil {
		t.Errorf("--help 应转发到 help: %v", err)
	}
}

func TestListCommandSignature(t *testing.T) {
	lc := &listCommand{}
	if lc.Signature() != "list" {
		t.Errorf("list 命令签名应为 'list', got %q", lc.Signature())
	}
}

func TestHelpCommandSignature(t *testing.T) {
	hc := &helpCommand{}
	if hc.Signature() != "help" {
		t.Errorf("help 命令签名应为 'help', got %q", hc.Signature())
	}
}

func TestMakeCommandSignatures(t *testing.T) {
	cmds := map[string]contracts.ConsoleCommand{
		"make:model":      &MakeModelCommand{},
		"make:controller": &MakeControllerCommand{},
		"make:provider":   &MakeProviderCommand{},
		"make:validator":  &MakeValidatorCommand{},
		"make:command":    &MakeCommandCommand{},
		"make:utils":      &MakeUtilsCommand{},
	}
	for expected, cmd := range cmds {
		if cmd.Signature() != expected {
			t.Errorf("%s: got %q", expected, cmd.Signature())
		}
	}
}

func TestTestCommandImplementsInterface(t *testing.T) {
	var _ contracts.ConsoleCommand = (*testCommand)(nil)
}

func TestRunSyncUnknownCommand(t *testing.T) {
	k := newKernel()
	err := k.RunSync([]string{"doesnotexist"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("未知命令应返回 not found 错误, got %v", err)
	}
}

func TestParseArgs(t *testing.T) {
	flags := []contracts.ConsoleFlag{
		&StringFlag{Name: "name", Aliases: []string{"n"}},
		&BoolFlag{Name: "force", Aliases: []string{"f"}},
	}
	opts, pos := parseArgs([]string{"pos1", "--name", "val", "-f"}, flags)
	if len(pos) != 1 || pos[0] != "pos1" {
		t.Errorf("位置参数: got %v, want [pos1]", pos)
	}
	if opts["name"] != "val" {
		t.Errorf("选项 name: got %q, want val", opts["name"])
	}
	if opts["force"] != "true" {
		t.Errorf("选项 force: got %q, want true", opts["force"])
	}
}

func TestParseArgsDefaults(t *testing.T) {
	flags := []contracts.ConsoleFlag{
		&StringFlag{Name: "env", Value: "dev"},
	}
	opts, pos := parseArgs([]string{}, flags)
	if len(pos) != 0 {
		t.Errorf("无位置参数: got %v", pos)
	}
	if opts["env"] != "dev" {
		t.Errorf("默认值: got %q, want dev", opts["env"])
	}
}

func TestRunDefaultSync(t *testing.T) {
	var executed bool

	k := newKernel()
	k.Register([]contracts.ConsoleCommand{
		&testCommand{
			sig: "heavy:task",
			execFn: func(ctx contracts.ConsoleContext) error {
				executed = true
				return nil
			},
		},
	})

	err := k.Run([]string{"heavy:task"})
	if err != nil || !executed {
		t.Error("默认同步: Run 应同步阻塞执行")
	}
}

func TestRunAsyncFlag(t *testing.T) {
	var executed atomic.Value
	executed.Store(false)

	k := newKernel()
	k.Register([]contracts.ConsoleCommand{
		&testCommand{
			sig: "heavy:task",
			execFn: func(ctx contracts.ConsoleContext) error {
				time.Sleep(50 * time.Millisecond)
				executed.Store(true)
				return nil
			},
		},
	})

	err := k.Run([]string{"heavy:task", "--async"})
	if err != nil || executed.Load().(bool) {
		t.Error("--async 标志: Run 应立即返回，命令不应已完成")
	}

	time.Sleep(100 * time.Millisecond)
	if !executed.Load().(bool) {
		t.Error("--async 标志: 命令应在后台完成")
	}
}

func TestRunSyncFlag(t *testing.T) {
	var executed bool
	k := newKernel()
	k.Register([]contracts.ConsoleCommand{
		&testCommand{
			sig: "sync:task",
			execFn: func(ctx contracts.ConsoleContext) error {
				executed = true
				return nil
			},
		},
	})

	err := k.Run([]string{"sync:task", "--sync"})
	if err != nil || !executed {
		t.Error("--sync 标志应同步阻塞执行")
	}
}

func TestRunSync(t *testing.T) {
	var executed bool
	k := newKernel()
	k.Register([]contracts.ConsoleCommand{
		&testCommand{
			sig: "runsync:test",
			execFn: func(ctx contracts.ConsoleContext) error {
				executed = true
				return nil
			},
		},
	})

	err := k.RunSync([]string{"runsync:test"})
	if err != nil || !executed {
		t.Error("RunSync 应同步阻塞执行")
	}
}

func TestCallSync(t *testing.T) {
	var executed bool
	k := newKernel()
	k.Register([]contracts.ConsoleCommand{
		&testCommand{
			sig: "callsync:test",
			execFn: func(ctx contracts.ConsoleContext) error {
				executed = true
				return nil
			},
		},
	})

	err := k.CallSync("callsync:test")
	if err != nil || !executed {
		t.Error("CallSync 应同步阻塞执行")
	}
}

func TestRunAsyncExplicit(t *testing.T) {
	var executed atomic.Value
	executed.Store(false)

	k := newKernel()
	k.Register([]contracts.ConsoleCommand{
		&testCommand{
			sig: "async:task",
			execFn: func(ctx contracts.ConsoleContext) error {
				executed.Store(true)
				return nil
			},
		},
	})

	k.RunAsync([]string{"async:task"})
	if executed.Load().(bool) {
		t.Error("RunAsync 应立即返回")
	}
	time.Sleep(50 * time.Millisecond)
	if !executed.Load().(bool) {
		t.Error("RunAsync 命令应在后台完成")
	}
}

func TestCallAsyncExplicit(t *testing.T) {
	var executed atomic.Value
	executed.Store(false)

	k := newKernel()
	k.Register([]contracts.ConsoleCommand{
		&testCommand{
			sig: "call:async",
			execFn: func(ctx contracts.ConsoleContext) error {
				executed.Store(true)
				return nil
			},
		},
	})

	k.CallAsync("call:async")
	time.Sleep(50 * time.Millisecond)
	if !executed.Load().(bool) {
		t.Error("CallAsync 命令应在后台完成")
	}
}

func TestStripFlag(t *testing.T) {
	found, filtered := stripFlag([]string{"db:migrate", "--sync", "--conn", "my"}, "--sync")
	if !found {
		t.Error("应检测到 --sync")
	}
	if len(filtered) != 3 {
		t.Errorf("应去除 --sync, got %v", filtered)
	}

	found, filtered = stripFlag([]string{"list"}, "--sync")
	if found {
		t.Error("无 --sync 时不应检测到")
	}
}

func TestRemoveFlag(t *testing.T) {
	result := removeFlag([]string{"a", "--x", "b"}, "--x")
	if len(result) != 2 || result[0] != "a" || result[1] != "b" {
		t.Errorf("removeFlag: got %v", result)
	}
}
