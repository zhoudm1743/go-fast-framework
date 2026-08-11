package contracts

// Fast 控制台服务契约。
type Fast interface {
	// Register 批量注册命令列表。
	Register(commands []ConsoleCommand)
	// Run 解析参数切片并分发执行（通常传入 os.Args[2:]）。默认异步执行。
	// 加 --sync 标志可切换为同步阻塞执行。
	Run(args []string) error
	// Call 以编程方式执行命令。默认异步，加 --sync 可同步。
	Call(command string) error
	// RunSync 同步执行命令，阻塞直到命令完成并返回错误。
	RunSync(args []string) error
	// CallSync 同 RunSync，以字符串形式传参。
	CallSync(command string) error
	// RunAsync 显式异步执行命令。
	RunAsync(args []string)
	// CallAsync 显式异步执行命令。
	CallAsync(command string)
}

// ConsoleCommand Fast 命令接口。
type ConsoleCommand interface {
	// Signature 唯一命令签名，如 "make:command" 或 "send:emails"。
	Signature() string
	// Description 简短描述，显示在 list 命令输出中。
	Description() string
	// Extend 声明命令的 Flags、Arguments、Category。
	Extend() CommandExtend
	// Handle 命令执行入口，返回非 nil error 时框架以退出码 1 结束。
	Handle(ctx ConsoleContext) error
}

// CommandExtend 命令扩展配置。
type CommandExtend struct {
	// Category 命令所属分类，用于 list 命令分组显示。
	Category string
	// Flags 选项列表（--key / -alias 形式）。
	Flags []ConsoleFlag
	// Arguments 位置参数列表（按顺序解析）。
	Arguments []ConsoleArgument
}

// ConsoleFlag 命令选项接口（--key value 或 -alias value 或 --bool-flag）。
type ConsoleFlag interface {
	FlagName() string
	FlagAliases() []string
	FlagUsage() string
	FlagDefaultStr() string
	IsBoolFlag() bool
}

// ConsoleArgument 命令位置参数接口。
type ConsoleArgument interface {
	ArgName() string
	ArgUsage() string
	IsArgRequired() bool
}

// ConsoleContext 命令执行上下文，传入 Handle 方法。
type ConsoleContext interface {
	// ─── 位置参数 ─────────────────────────────────────────────────
	// Argument 按索引（0-based）返回位置参数值，越界返回空字符串。
	Argument(index int) string
	// Arguments 返回全部位置参数。
	Arguments() []string

	// ─── 选项（Flag）─────────────────────────────────────────────
	// Option 返回选项字符串值，未传则返回默认值。
	Option(key string) string
	// OptionBool 返回选项布尔值。
	OptionBool(key string) bool

	// ─── 输出 ──────────────────────────────────────────────────────
	Line(msg string)
	Info(msg string)
	Comment(msg string)
	Warning(msg string)
	Error(msg string)
	NewLine(count ...int)

	// ─── 交互式输入 ────────────────────────────────────────────────
	// Ask 提示用户输入一行文本。
	Ask(question string, opts ...AskOption) (string, error)
	// Secret 提示用户输入敏感信息（密码等）。
	Secret(question string, opts ...SecretOption) (string, error)
	// Confirm 提示用户确认（y/N），返回 bool。
	Confirm(question string, opts ...ConfirmOption) bool
	// Choice 单选，返回所选项的 Key。
	Choice(question string, choices []ConsoleChoice, opts ...ChoiceOption) (string, error)
	// MultiSelect 多选，返回所有选中项的 Key 切片。
	MultiSelect(question string, choices []ConsoleChoice, opts ...MultiSelectOption) ([]string, error)
}

// ConsoleChoice 单选 / 多选条目。
type ConsoleChoice struct {
	Key      string
	Value    string
	Selected bool // 是否默认选中（列表展示用）
}

// AskOption Ask 配置项。
type AskOption struct {
	Default  string
	Validate func(string) error
}

// SecretOption Secret 配置项。
type SecretOption struct {
	Default  string
	Validate func(string) error
}

// ConfirmOption Confirm 配置项。
type ConfirmOption struct {
	Default     bool   // 默认值（直接回车时采用）
	Affirmative string // 肯定回答文本，默认 "yes"
	Negative    string // 否定回答文本，默认 "no"
}

// ChoiceOption Choice 配置项。
type ChoiceOption struct {
	Default  string
	Validate func(string) error
}

// MultiSelectOption MultiSelect 配置项。
type MultiSelectOption struct {
	Default  []string
	Validate func([]string) error
}
