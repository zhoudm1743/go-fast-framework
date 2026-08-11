package fast

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// MakeProviderCommand make:provider 命令
// 用法：go run . fast make:provider RedisServiceProvider
type MakeProviderCommand struct{}

func (c *MakeProviderCommand) Signature() string   { return "make:provider" }
func (c *MakeProviderCommand) Description() string { return "创建服务提供者" }

func (c *MakeProviderCommand) Extend() contracts.CommandExtend {
	return contracts.CommandExtend{
		Category: "make",
		Arguments: []contracts.ConsoleArgument{
			&StringArgument{
				Name:     "name",
				Usage:    "提供者名（PascalCase），如 RedisServiceProvider",
				Required: true,
			},
		},
	}
}

func (c *MakeProviderCommand) Handle(ctx contracts.ConsoleContext) error {
	name := toPascalCase(ctx.Argument(0))
	if name == "" {
		ctx.Error("请提供服务提供者名称，例如：make:provider RedisServiceProvider")
		return fmt.Errorf("missing provider name")
	}

	// 确保以 ServiceProvider 结尾
	if !strings.HasSuffix(name, "ServiceProvider") {
		name += "ServiceProvider"
	}

	wd, _ := os.Getwd()
	module := readGoMod()

	// 去掉 ServiceProvider 后缀作为文件名前缀
	baseName := stripSuffix(name, "ServiceProvider")
	fileName := toSnakeCase(baseName) + "_service_provider.go"
	filePath := filepath.Join(wd, "app", "providers", fileName)

	content := buildProviderContent(module, name)

	if err := writeGeneratedFile(filePath, content); err != nil {
		ctx.Error(err.Error())
		return err
	}

	ctx.Info(fmt.Sprintf("✓ 服务提供者已创建：%s", relPath(filePath)))
	ctx.Comment(fmt.Sprintf("  记得在 bootstrap/app.go 的 providers() 中注册 &providers.%s{}", name))
	return nil
}

func buildProviderContent(module, structName string) string {
	return strings.Join([]string{
		"package providers",
		"",
		"import (",
		`	"github.com/zhoudm1743/go-fast-framework/foundation"`,
		"",
		`	// 在此引入所需依赖`,
		`	_ "github.com/zhoudm1743/go-fast-framework/facades"`,
		")",
		"",
		fmt.Sprintf("// %s 服务提供者。", structName),
		fmt.Sprintf("type %s struct{}", structName),
		"",
		"// Register 将服务绑定到容器。此阶段不可使用其他服务。",
		fmt.Sprintf("func (sp *%s) Register(app foundation.Application) {", structName),
		"\t// app.Singleton(\"my-service\", func(app foundation.Application) (any, error) {",
		"\t//     return NewMyService(), nil",
		"\t// })",
		"}",
		"",
		"// Boot 引导服务。所有 Provider 均已 Register 完成，可安全使用容器中的服务。",
		fmt.Sprintf("func (sp *%s) Boot(_ foundation.Application) error {", structName),
		"\treturn nil",
		"}",
		"",
	}, "\n")
}
