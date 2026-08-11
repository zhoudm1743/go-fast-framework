package fast

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// ANSI 颜色常量（Windows Terminal / PowerShell 均支持 VT100）
const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorCyan   = "\033[36m"
)

// consoleContext 实现 contracts.ConsoleContext。
type consoleContext struct {
	args    []string          // 位置参数
	options map[string]string // 解析后的选项
	reader  *bufio.Reader
}

func newConsoleContext(args []string, options map[string]string) contracts.ConsoleContext {
	return &consoleContext{
		args:    args,
		options: options,
		reader:  bufio.NewReader(os.Stdin),
	}
}

// ─── 位置参数 ─────────────────────────────────────────────────────────────────

func (c *consoleContext) Argument(index int) string {
	if index < 0 || index >= len(c.args) {
		return ""
	}
	return c.args[index]
}

func (c *consoleContext) Arguments() []string {
	return c.args
}

// ─── 选项 ─────────────────────────────────────────────────────────────────────

func (c *consoleContext) Option(key string) string {
	return c.options[key]
}

func (c *consoleContext) OptionBool(key string) bool {
	v := strings.ToLower(c.options[key])
	return v == "true" || v == "1" || v == "yes"
}

// ─── 输出 ─────────────────────────────────────────────────────────────────────

func (c *consoleContext) Line(msg string)    { fmt.Println(msg) }
func (c *consoleContext) Info(msg string)    { fmt.Printf("%s%s%s\n", colorGreen, msg, colorReset) }
func (c *consoleContext) Comment(msg string) { fmt.Printf("%s%s%s\n", colorCyan, msg, colorReset) }
func (c *consoleContext) Warning(msg string) { fmt.Printf("%s%s%s\n", colorYellow, msg, colorReset) }
func (c *consoleContext) Error(msg string) {
	fmt.Fprintf(os.Stderr, "%s%s%s\n", colorRed, msg, colorReset)
}

func (c *consoleContext) NewLine(count ...int) {
	n := 1
	if len(count) > 0 && count[0] > 0 {
		n = count[0]
	}
	for i := 0; i < n; i++ {
		fmt.Println()
	}
}

// ─── 交互式输入 ───────────────────────────────────────────────────────────────

func (c *consoleContext) Ask(question string, opts ...contracts.AskOption) (string, error) {
	opt := contracts.AskOption{}
	if len(opts) > 0 {
		opt = opts[0]
	}

	prompt := question
	if opt.Default != "" {
		prompt = fmt.Sprintf("%s [%s]", question, opt.Default)
	}
	fmt.Print(prompt + ": ")

	input, err := c.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	input = strings.TrimRight(input, "\r\n")

	if input == "" && opt.Default != "" {
		input = opt.Default
	}

	if opt.Validate != nil {
		if verr := opt.Validate(input); verr != nil {
			c.Error(verr.Error())
			return c.Ask(question, opts...)
		}
	}

	return input, nil
}

func (c *consoleContext) Secret(question string, opts ...contracts.SecretOption) (string, error) {
	opt := contracts.SecretOption{}
	if len(opts) > 0 {
		opt = opts[0]
	}

	// 注意：此实现不隐藏输入。如需隐藏，请集成 golang.org/x/term.ReadPassword。
	fmt.Print(question + ": ")

	input, err := c.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	input = strings.TrimRight(input, "\r\n")

	if input == "" && opt.Default != "" {
		input = opt.Default
	}

	if opt.Validate != nil {
		if verr := opt.Validate(input); verr != nil {
			c.Error(verr.Error())
			return c.Secret(question, opts...)
		}
	}

	return input, nil
}

func (c *consoleContext) Confirm(question string, opts ...contracts.ConfirmOption) bool {
	opt := contracts.ConfirmOption{Affirmative: "yes", Negative: "no"}
	if len(opts) > 0 {
		o := opts[0]
		if o.Affirmative != "" {
			opt.Affirmative = o.Affirmative
		}
		if o.Negative != "" {
			opt.Negative = o.Negative
		}
		opt.Default = o.Default
	}

	defaultHint := "N"
	if opt.Default {
		defaultHint = "Y"
	}

	fmt.Printf("%s [y/N, default=%s]: ", question, defaultHint)
	input, _ := c.reader.ReadString('\n')
	input = strings.TrimRight(input, "\r\n")
	input = strings.TrimSpace(input)

	if input == "" {
		return opt.Default
	}

	lower := strings.ToLower(input)
	return lower == "y" || lower == "yes" || lower == strings.ToLower(opt.Affirmative)
}

func (c *consoleContext) Choice(question string, choices []contracts.ConsoleChoice, opts ...contracts.ChoiceOption) (string, error) {
	opt := contracts.ChoiceOption{}
	if len(opts) > 0 {
		opt = opts[0]
	}

	fmt.Println(question)
	for i, ch := range choices {
		mark := ""
		if ch.Selected {
			mark = " *"
		}
		fmt.Printf("  [%d] %s%s\n", i+1, ch.Value, mark)
	}

	if opt.Default != "" {
		fmt.Printf("请选择 [%s]: ", opt.Default)
	} else {
		fmt.Print("请选择：")
	}

	input, err := c.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	input = strings.TrimSpace(strings.TrimRight(input, "\r\n"))

	if input == "" && opt.Default != "" {
		return opt.Default, nil
	}

	// 数字索引
	if idx, err := strconv.Atoi(input); err == nil && idx >= 1 && idx <= len(choices) {
		key := choices[idx-1].Key
		if opt.Validate != nil {
			if verr := opt.Validate(key); verr != nil {
				c.Error(verr.Error())
				return c.Choice(question, choices, opts...)
			}
		}
		return key, nil
	}

	// key / value 匹配
	for _, ch := range choices {
		if ch.Key == input || ch.Value == input {
			return ch.Key, nil
		}
	}

	c.Error("输入无效，请重新选择。")
	return c.Choice(question, choices, opts...)
}

func (c *consoleContext) MultiSelect(question string, choices []contracts.ConsoleChoice, opts ...contracts.MultiSelectOption) ([]string, error) {
	opt := contracts.MultiSelectOption{}
	if len(opts) > 0 {
		opt = opts[0]
	}

	fmt.Println(question + " （请输入编号，逗号分隔，如 1,3）")
	for i, ch := range choices {
		mark := ""
		if ch.Selected {
			mark = " *"
		}
		fmt.Printf("  [%d] %s%s\n", i+1, ch.Value, mark)
	}

	if len(opt.Default) > 0 {
		fmt.Printf("请选择 [%s]: ", strings.Join(opt.Default, ","))
	} else {
		fmt.Print("请选择：")
	}

	input, err := c.reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	input = strings.TrimSpace(strings.TrimRight(input, "\r\n"))

	if input == "" && len(opt.Default) > 0 {
		return opt.Default, nil
	}

	var selected []string
	for _, part := range strings.Split(input, ",") {
		part = strings.TrimSpace(part)
		if idx, err := strconv.Atoi(part); err == nil && idx >= 1 && idx <= len(choices) {
			selected = append(selected, choices[idx-1].Key)
			continue
		}
		for _, ch := range choices {
			if ch.Key == part || ch.Value == part {
				selected = append(selected, ch.Key)
				break
			}
		}
	}

	if opt.Validate != nil {
		if verr := opt.Validate(selected); verr != nil {
			c.Error(verr.Error())
			return c.MultiSelect(question, choices, opts...)
		}
	}

	return selected, nil
}
