package fast

import (
	"fmt"
	"strings"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// ─── 选项（Flag）类型 ─────────────────────────────────────────────────────────

// StringFlag 字符串选项，对应 --key value 或 -alias value。
type StringFlag struct {
	Name    string
	Value   string   // 默认值
	Aliases []string // 短别名，如 []string{"l"} 对应 -l
	Usage   string
}

var _ contracts.ConsoleFlag = (*StringFlag)(nil)

func (f *StringFlag) FlagName() string       { return f.Name }
func (f *StringFlag) FlagAliases() []string  { return f.Aliases }
func (f *StringFlag) FlagUsage() string      { return f.Usage }
func (f *StringFlag) FlagDefaultStr() string { return f.Value }
func (f *StringFlag) IsBoolFlag() bool       { return false }

// BoolFlag 布尔选项，只需声明 --flag 即为 true，不需要值。
type BoolFlag struct {
	Name    string
	Aliases []string
	Usage   string
}

var _ contracts.ConsoleFlag = (*BoolFlag)(nil)

func (f *BoolFlag) FlagName() string       { return f.Name }
func (f *BoolFlag) FlagAliases() []string  { return f.Aliases }
func (f *BoolFlag) FlagUsage() string      { return f.Usage }
func (f *BoolFlag) FlagDefaultStr() string { return "false" }
func (f *BoolFlag) IsBoolFlag() bool       { return true }

// IntFlag 整数选项。
type IntFlag struct {
	Name    string
	Value   int
	Aliases []string
	Usage   string
}

var _ contracts.ConsoleFlag = (*IntFlag)(nil)

func (f *IntFlag) FlagName() string       { return f.Name }
func (f *IntFlag) FlagAliases() []string  { return f.Aliases }
func (f *IntFlag) FlagUsage() string      { return f.Usage }
func (f *IntFlag) FlagDefaultStr() string { return fmt.Sprintf("%d", f.Value) }
func (f *IntFlag) IsBoolFlag() bool       { return false }

// StringSliceFlag 字符串切片选项，可多次传入同一 key 累积。
type StringSliceFlag struct {
	Name    string
	Value   []string
	Aliases []string
	Usage   string
}

var _ contracts.ConsoleFlag = (*StringSliceFlag)(nil)

func (f *StringSliceFlag) FlagName() string       { return f.Name }
func (f *StringSliceFlag) FlagAliases() []string  { return f.Aliases }
func (f *StringSliceFlag) FlagUsage() string      { return f.Usage }
func (f *StringSliceFlag) FlagDefaultStr() string { return strings.Join(f.Value, ",") }
func (f *StringSliceFlag) IsBoolFlag() bool       { return false }

// ─── 参数（Argument）类型 ──────────────────────────────────────────────────────

// StringArgument 字符串位置参数。
type StringArgument struct {
	Name     string
	Usage    string
	Required bool
}

var _ contracts.ConsoleArgument = (*StringArgument)(nil)

func (a *StringArgument) ArgName() string     { return a.Name }
func (a *StringArgument) ArgUsage() string    { return a.Usage }
func (a *StringArgument) IsArgRequired() bool { return a.Required }
