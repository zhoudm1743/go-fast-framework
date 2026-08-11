package fast

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// MakeValidatorCommand make:validator 命令
// 用法：go run . fast make:validator Phone
// 生成自定义验证规则文件，规则名即为命令参数（小写），
// 注册方式：在服务提供者 Boot 中调用 facades.App().MustMake("validator").(contracts.Validation).RegisterRule(...)
type MakeValidatorCommand struct{}

func (c *MakeValidatorCommand) Signature() string   { return "make:validator" }
func (c *MakeValidatorCommand) Description() string { return "创建自定义验证规则" }

func (c *MakeValidatorCommand) Extend() contracts.CommandExtend {
	return contracts.CommandExtend{
		Category: "make",
		Arguments: []contracts.ConsoleArgument{
			&StringArgument{
				Name:     "name",
				Usage:    "规则名（PascalCase），如 Phone / UniqueEmail",
				Required: true,
			},
		},
	}
}

func (c *MakeValidatorCommand) Handle(ctx contracts.ConsoleContext) error {
	name := toPascalCase(ctx.Argument(0))
	if name == "" {
		ctx.Error("请提供验证规则名称，例如：make:validator Phone")
		return fmt.Errorf("missing validator name")
	}

	// 去掉可能的 Rule 后缀再重新加上
	baseName := stripSuffix(name, "Rule")
	structName := baseName + "Rule"
	ruleName := strings.ToLower(toSnakeCase(baseName))

	wd, _ := os.Getwd()
	module := readGoMod()
	fileName := toSnakeCase(baseName) + "_rule.go"
	filePath := filepath.Join(wd, "app", "rules", fileName)

	content := buildValidatorContent(module, structName, ruleName)

	if err := writeGeneratedFile(filePath, content); err != nil {
		ctx.Error(err.Error())
		return err
	}

	ctx.Info(fmt.Sprintf("✓ 验证规则已创建：%s", relPath(filePath)))
	ctx.Comment(fmt.Sprintf("  规则标签名：binding:\"%s\"", ruleName))
	ctx.Comment("  注册规则：在服务提供者 Boot 中调用：")
	ctx.Comment(fmt.Sprintf("    facades.App().MustMake(\"validator\").(contracts.Validation).RegisterRule(&rules.%s{})", structName))
	return nil
}

func buildValidatorContent(module, structName, ruleName string) string {
	return strings.Join([]string{
		"package rules",
		"",
		"import (",
		`	"github.com/go-playground/validator/v10"`,
		")",
		"",
		fmt.Sprintf("// %s 自定义验证规则。", structName),
		fmt.Sprintf("// 在 binding tag 中使用：binding:\"%s\"", ruleName),
		"//",
		"// 注册方式（在某个 ServiceProvider.Boot 中）：",
		"//",
		"//   v := app.MustMake(\"validator\").(contracts.Validation)",
		fmt.Sprintf("//   v.RegisterRule(&rules.%s{})", structName),
		fmt.Sprintf("type %s struct{}", structName),
		"",
		"// Rule 返回该规则在 binding tag 中使用的名称。",
		fmt.Sprintf("func (r *%s) Rule() string {", structName),
		fmt.Sprintf("\treturn \"%s\"", ruleName),
		"}",
		"",
		"// Validate 验证逻辑。",
		"// fl.Field() 可获取当前字段值；fl.Param() 可获取规则参数。",
		fmt.Sprintf("func (r *%s) Validate(fl validator.FieldLevel) bool {", structName),
		"\t// TODO: 实现验证逻辑",
		"\t// value := fl.Field().String()",
		"\treturn true",
		"}",
		"",
		"// Message 当验证不通过时返回的错误消息模板。",
		"// :attribute 会被框架替换为字段名。",
		fmt.Sprintf("func (r *%s) Message() string {", structName),
		fmt.Sprintf("\treturn \"The :attribute is not a valid %s.\"", ruleName),
		"}",
		"",
		fmt.Sprintf("// 确保 %s 实现了 validator.Func 兼容接口（编译期检查）。", structName),
		"// go-playground/validator 通过 RegisterValidation 注册，",
		"// 以下仅作为结构组织参考，实际注册由框架的 RegisterRule 方法处理。",
		fmt.Sprintf("var _ = (*%s)(nil)", structName),
		"",
		"// RegistrationFunc 将本规则注册到 go-playground/validator 的适配函数。",
		"// 框架的 validatorImpl.RegisterRule 会检测此方法并调用。",
		fmt.Sprintf("func (r *%s) RegistrationFunc() validator.Func {", structName),
		"\treturn r.Validate",
		"}",
		"",
		"// 确保编译期导入 module（防止 IDE 误报）",
		fmt.Sprintf("var _ = \"%s\"", module),
		"",
	}, "\n")
}
