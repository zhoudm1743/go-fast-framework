package fast

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// MakeModelCommand make:model 命令
// 用法：go run . fast make:model Post [--soft-delete]
type MakeModelCommand struct{}

func (c *MakeModelCommand) Signature() string   { return "make:model" }
func (c *MakeModelCommand) Description() string { return "创建模型" }

func (c *MakeModelCommand) Extend() contracts.CommandExtend {
	return contracts.CommandExtend{
		Category: "make",
		Flags: []contracts.ConsoleFlag{
			&BoolFlag{
				Name:    "soft-delete",
				Aliases: []string{"s"},
				Usage:   "使用 ModelWithSoftDelete（含软删除 deleted_at 字段）",
			},
		},
		Arguments: []contracts.ConsoleArgument{
			&StringArgument{
				Name:     "name",
				Usage:    "模型名（PascalCase），如 Post / UserProfile",
				Required: true,
			},
		},
	}
}

func (c *MakeModelCommand) Handle(ctx contracts.ConsoleContext) error {
	name := ctx.Argument(0)
	if name == "" {
		ctx.Error("请提供模型名称，例如：make:model Post")
		return fmt.Errorf("missing model name")
	}

	name = toPascalCase(name)
	softDelete := ctx.OptionBool("soft-delete")

	wd, _ := os.Getwd()
	fileName := toSnakeCase(name) + ".go"
	filePath := filepath.Join(wd, "app", "models", fileName)
	module := readGoMod()

	content := buildModelContent(module, name, softDelete)

	if err := writeGeneratedFile(filePath, content); err != nil {
		ctx.Error(err.Error())
		return err
	}

	ctx.Info(fmt.Sprintf("✓ 模型已创建：%s", relPath(filePath)))
	return nil
}

func buildModelContent(module, name string, softDelete bool) string {
	baseType := "database.Model"
	if softDelete {
		baseType = "database.ModelWithSoftDelete"
	}

	return strings.Join([]string{
		"package models",
		"",
		`import "github.com/zhoudm1743/go-fast-framework/database"`,
		"",
		fmt.Sprintf("// %s 模型。", name),
		fmt.Sprintf("// 嵌入 %s 自动获得 UUID v7 主键、CreatedAt、UpdatedAt。", baseType),
		fmt.Sprintf("type %s struct {", name),
		fmt.Sprintf("\t%s", baseType),
		"\t// TODO: 在此添加字段",
		"\t// Name string `gorm:\"size:100;not null\" json:\"name\"`",
		"}",
		"",
	}, "\n")
}
