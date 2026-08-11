package fast

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// MakeCommandCommand make:command 命令
// 用法：go run . fast make:command SendEmails [--signature send:emails] [--category send]
// 在 app/console/commands/ 下生成一个实现 contracts.ConsoleCommand 接口的命令文件。
type MakeCommandCommand struct{}

func (c *MakeCommandCommand) Signature() string   { return "make:command" }
func (c *MakeCommandCommand) Description() string { return "创建自定义命令" }

func (c *MakeCommandCommand) Extend() contracts.CommandExtend {
	return contracts.CommandExtend{
		Category: "make",
		Flags: []contracts.ConsoleFlag{
			&StringFlag{
				Name:    "signature",
				Aliases: []string{"s"},
				Usage:   "命令签名（默认由名称派生，如 send:emails）",
			},
			&StringFlag{
				Name:    "category",
				Aliases: []string{"c"},
				Usage:   "命令分类，显示在 list 中（可选）",
			},
		},
		Arguments: []contracts.ConsoleArgument{
			&StringArgument{
				Name:     "name",
				Usage:    "命令结构体名（PascalCase），如 SendEmails",
				Required: true,
			},
		},
	}
}

func (c *MakeCommandCommand) Handle(ctx contracts.ConsoleContext) error {
	name := toPascalCase(ctx.Argument(0))
	if name == "" {
		ctx.Error("请提供命令名称，例如：make:command SendEmails")
		return fmt.Errorf("missing command name")
	}

	// 自动补全 Command 后缀
	structName := name
	if !strings.HasSuffix(strings.ToLower(structName), "command") {
		structName += "Command"
	}

	// 签名：优先用 --signature，否则从结构体名派生
	signature := ctx.Option("signature")
	if signature == "" {
		signature = deriveSignature(structName)
	}

	category := ctx.Option("category")
	module := readGoMod()

	wd, _ := os.Getwd()
	baseName := stripSuffix(structName, "Command")
	fileName := toSnakeCase(baseName) + ".go"
	filePath := filepath.Join(wd, "app", "console", "commands", fileName)

	content := buildCommandContent(module, structName, signature, category)

	if err := writeGeneratedFile(filePath, content); err != nil {
		ctx.Error(err.Error())
		return err
	}

	ctx.Info(fmt.Sprintf("✓ 命令已创建：%s", relPath(filePath)))
	ctx.Comment(fmt.Sprintf("  签名：%s", signature))
	ctx.Comment("  记得在 bootstrap/commands.go 的 Commands() 中注册：")
	ctx.Comment(fmt.Sprintf("    &commands.%s{},", structName))
	return nil
}

// deriveSignature 从结构体名派生命令签名。
// SendEmailsCommand → send:emails
// MigrateUserDataCommand → migrate:user-data
func deriveSignature(structName string) string {
	// 去掉 Command 后缀
	base := stripSuffix(structName, "Command")
	snake := toSnakeCase(base) // send_emails / migrate_user_data

	// 将第一个 _ 替换为 :，其余替换为 -
	// send_emails → send:emails
	// migrate_user_data → migrate:user-data
	parts := strings.SplitN(snake, "_", 2)
	if len(parts) == 1 {
		return parts[0]
	}
	return parts[0] + ":" + strings.ReplaceAll(parts[1], "_", "-")
}

func buildCommandContent(module, structName, signature, category string) string {
	catLine := ""
	if category != "" {
		catLine = fmt.Sprintf("\t\tCategory: %q,\n", category)
	}

	return strings.Join([]string{
		"package commands",
		"",
		"import (",
		`	"github.com/zhoudm1743/go-fast-framework/fast"`,
		`	"github.com/zhoudm1743/go-fast-framework/contracts"`,
		")",
		"",
		fmt.Sprintf("// %s is an Fast command.", structName),
		fmt.Sprintf("// Run: go run . fast %s", signature),
		fmt.Sprintf("type %s struct{}", structName),
		"",
		"// Signature returns the unique command name used to invoke it.",
		fmt.Sprintf("func (c *%s) Signature() string { return %q }", structName, signature),
		"",
		"// Description returns a short description shown in list.",
		fmt.Sprintf("func (c *%s) Description() string {", structName),
		fmt.Sprintf("\treturn %q", "TODO: describe this command"),
		"}",
		"",
		"// Extend declares flags, arguments and category.",
		fmt.Sprintf("func (c *%s) Extend() contracts.CommandExtend {", structName),
		"\treturn contracts.CommandExtend{",
		catLine + "\t\t// Flags: []contracts.ConsoleFlag{",
		"\t\t//     &fast.StringFlag{Name: \"output\", Aliases: []string{\"o\"}, Usage: \"output path\"},",
		"\t\t//     &fast.BoolFlag{Name: \"force\", Usage: \"force overwrite\"},",
		"\t\t// },",
		"\t\t// Arguments: []contracts.ConsoleArgument{",
		"\t\t//     &fast.StringArgument{Name: \"name\", Usage: \"target name\", Required: true},",
		"\t\t// },",
		"\t}",
		"}",
		"",
		"// Handle contains the command logic.",
		fmt.Sprintf("func (c *%s) Handle(ctx contracts.ConsoleContext) error {", structName),
		"\t// name   := ctx.Argument(0)",
		"\t// output := ctx.Option(\"output\")",
		"\t// force  := ctx.OptionBool(\"force\")",
		"\t_ = fast.StringFlag{} // keep fast import",
		"\tctx.Info(\"TODO: implement command logic\")",
		"\treturn nil",
		"}",
		"",
	}, "\n")
}
