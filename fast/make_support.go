package fast

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// MakeUtilsCommand make:utils令
// 用法：go run . fast make:utils Jwt
// 在 framework/utils/ 中生成新的工具集文件，遵循 XxxUtil = xxxUtil{} 规范。
type MakeUtilsCommand struct{}

func (c *MakeUtilsCommand) Signature() string { return "make:utils" }
func (c *MakeUtilsCommand) Description() string {
	return "在 framework/utils 中创建工具类"
}

func (c *MakeUtilsCommand) Extend() contracts.CommandExtend {
	return contracts.CommandExtend{
		Category: "make",
		Arguments: []contracts.ConsoleArgument{
			&StringArgument{
				Name:     "name",
				Usage:    "工具名（PascalCase），如 Jwt / Http / Encrypt",
				Required: true,
			},
		},
	}
}

func (c *MakeUtilsCommand) Handle(ctx contracts.ConsoleContext) error {
	name := toPascalCase(ctx.Argument(0))
	if name == "" {
		ctx.Error("请提供工具名称，例如：make:utils Jwt")
		return fmt.Errorf("missing utils name")
	}

	// 去掉用户可能带的 Util 后缀，内部统一处理
	baseName := stripSuffix(name, "Util")

	wd, _ := os.Getwd()
	fileName := toSnakeCase(baseName) + ".go"
	filePath := filepath.Join(wd, "framework", "utils", fileName)

	content := buildUtilsContent(baseName)

	if err := writeGeneratedFile(filePath, content); err != nil {
		ctx.Error(err.Error())
		return err
	}

	ctx.Info(fmt.Sprintf("✓ 工具已创建：%s", relPath(filePath)))
	ctx.Comment(fmt.Sprintf("  使用方式：utils.%sUtil.YourMethod()", baseName))
	return nil
}

func buildUtilsContent(baseName string) string {
	exportVar := baseName + "Util"
	structType := strings.ToLower(baseName[:1]) + baseName[1:] + "Util"

	return strings.Join([]string{
		"package utils",
		"",
		fmt.Sprintf("var %s = %s{}", exportVar, structType),
		"",
		fmt.Sprintf("type %s struct{}", structType),
		"",
		"// Add your methods below.",
		"//",
		"// Example:",
		fmt.Sprintf("// func (r %s) Foo(s string) string {", structType),
		"//     return s",
		"// }",
		"",
	}, "\n")
}
