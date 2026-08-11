package fast

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// MakeControllerCommand make:controller 命令
// 用法：go run . fast make:controller Post [--group admin|app] [--with-request]
// 默认同时生成 controller + request 文件。
type MakeControllerCommand struct{}

func (c *MakeControllerCommand) Signature() string { return "make:controller" }
func (c *MakeControllerCommand) Description() string {
	return "创建控制器（同时生成请求文件）"
}

func (c *MakeControllerCommand) Extend() contracts.CommandExtend {
	return contracts.CommandExtend{
		Category: "make",
		Flags: []contracts.ConsoleFlag{
			&StringFlag{
				Name:    "group",
				Value:   "app",
				Aliases: []string{"g"},
				Usage:   "路由分组：admin 或 app（默认 app）",
			},
			&BoolFlag{
				Name:  "no-request",
				Usage: "跳过生成请求文件",
			},
		},
		Arguments: []contracts.ConsoleArgument{
			&StringArgument{
				Name:     "name",
				Usage:    "控制器名（PascalCase），如 Post / UserProfile",
				Required: true,
			},
		},
	}
}

func (c *MakeControllerCommand) Handle(ctx contracts.ConsoleContext) error {
	name := toPascalCase(ctx.Argument(0))
	if name == "" {
		ctx.Error("请提供控制器名称，例如：make:controller Post")
		return fmt.Errorf("missing controller name")
	}
	// 自动去掉用户可能带的 Controller 后缀
	baseName := stripSuffix(name, "Controller")

	group := ctx.Option("group")
	if group != "admin" && group != "app" {
		ctx.Error("--group 只能是 admin 或 app")
		return fmt.Errorf("invalid group: %s", group)
	}
	noRequest := ctx.OptionBool("no-request")

	wd, _ := os.Getwd()
	module := readGoMod()

	// ── 控制器文件 ──────────────────────────────────────────────────
	ctrlDir := filepath.Join(wd, "app", "http", group, "controllers")
	ctrlFile := filepath.Join(ctrlDir, toSnakeCase(baseName)+"_controller.go")
	ctrlContent := buildControllerContent(module, baseName, group, !noRequest)

	if err := writeGeneratedFile(ctrlFile, ctrlContent); err != nil {
		ctx.Error(err.Error())
		return err
	}
	ctx.Info(fmt.Sprintf("✓ 控制器已创建：%s", relPath(ctrlFile)))

	// ── 请求文件 ────────────────────────────────────────────────────
	if !noRequest {
		reqDir := filepath.Join(wd, "app", "http", group, "requests")
		reqFile := filepath.Join(reqDir, toSnakeCase(baseName)+".go")
		reqContent := buildRequestContent(baseName)

		if err := writeGeneratedFile(reqFile, reqContent); err != nil {
			ctx.Error(err.Error())
			return err
		}
		ctx.Info(fmt.Sprintf("✓ 请求已创建：%s", relPath(reqFile)))
	}

	return nil
}

// ─── 模板 ─────────────────────────────────────────────────────────────────────

func buildControllerContent(module, baseName, group string, withRequest bool) string {
	structName := baseName + "Controller"
	routePath := toRoutePath(baseName)
	snake := toSnakeCase(baseName)
	reqPkg := fmt.Sprintf(`%s/app/http/%s/requests`, module, group)

	imports := []string{
		`"net/http"`,
		`""`,
		`"github.com/zhoudm1743/go-fast-framework/contracts"`,
		`"github.com/zhoudm1743/go-fast-framework/facades"`,
	}
	if withRequest {
		imports = append(imports,
			fmt.Sprintf(`requests "%s"`, reqPkg),
		)
	}

	// 仅保留非空 import 行
	var importLines []string
	for _, imp := range imports {
		if imp == `""` {
			importLines = append(importLines, "")
		} else {
			importLines = append(importLines, "\t"+imp)
		}
	}

	lines := []string{
		"package controllers",
		"",
		"import (",
	}
	lines = append(lines, importLines...)
	lines = append(lines,
		")",
		"",
		fmt.Sprintf("// %s %s 控制器。", structName, baseName),
		fmt.Sprintf("type %s struct{}", structName),
		"",
		"// Prefix 路由前缀（实现 contracts.Prefixer）。",
		fmt.Sprintf("func (c *%s) Prefix() string { return \"%s\" }", structName, routePath),
		"",
		"// Boot 声明路由（实现 contracts.Controller）。",
		fmt.Sprintf("func (c *%s) Boot(r contracts.Route) {", structName),
		"\tr.Get(\"/\", c.Index)         // GET    "+routePath,
		"\tr.Get(\"/:id\", c.Show)       // GET    "+routePath+"/:id",
		"\tr.Post(\"/\", c.Store)        // POST   "+routePath,
		"\tr.Put(\"/:id\", c.Update)     // PUT    "+routePath+"/:id",
		"\tr.Delete(\"/:id\", c.Destroy) // DELETE "+routePath+"/:id",
		"}",
		"",
	)

	// 请求变量名
	reqVar := func(t string) string {
		if withRequest {
			return fmt.Sprintf("requests.%s%sRequest", t, baseName)
		}
		return "struct{}"
	}
	_ = reqVar // suppress unused warning for non-request mode

	listMethod := buildMethod(structName, "Index", "ctx contracts.Context",
		withRequest,
		fmt.Sprintf("\tvar req %s", reqVar("List")),
		"\tif err := ctx.Bind(&req); err != nil {",
		"\t\treturn ctx.Response().Validation(err)",
		"\t}",
		"\t_ = req",
		"\t_ = http.StatusOK",
		"\t_ = facades.Log()",
		"\t// TODO: 实现列表查询",
		"\treturn ctx.Response().Success(nil)",
	)

	showMethod := buildMethod(structName, "Show", "ctx contracts.Context",
		false,
		"\tid := ctx.Param(\"id\")",
		"\t_ = id",
		"\t// TODO: 实现详情查询",
		"\treturn ctx.Response().Success(nil)",
	)

	storeMethod := buildMethod(structName, "Store", "ctx contracts.Context",
		withRequest,
		fmt.Sprintf("\tvar req %s", reqVar("Create")),
		"\tif err := ctx.Bind(&req); err != nil {",
		"\t\treturn ctx.Response().Validation(err)",
		"\t}",
		"\t_ = req",
		"\t// TODO: 实现创建",
		"\treturn ctx.Response().Created(nil)",
	)

	updateMethod := buildMethod(structName, "Update", "ctx contracts.Context",
		withRequest,
		fmt.Sprintf("\tvar req %s", reqVar("Update")),
		"\tif err := ctx.Bind(&req); err != nil {",
		"\t\treturn ctx.Response().Validation(err)",
		"\t}",
		"\t_ = req",
		"\t// TODO: 实现更新",
		"\treturn ctx.Response().Success(nil)",
	)

	destroyMethod := buildMethod(structName, "Destroy", "ctx contracts.Context",
		false,
		"\tid := ctx.Param(\"id\")",
		"\t_ = id",
		"\t// TODO: 实现删除",
		"\treturn ctx.Response().Success(nil)",
	)

	_ = snake
	lines = append(lines, listMethod, showMethod, storeMethod, updateMethod, destroyMethod)

	return strings.Join(lines, "\n")
}

func buildMethod(receiver, method, params string, withRequest bool, body ...string) string {
	lines := []string{
		fmt.Sprintf("// %s 处理器。", method),
		fmt.Sprintf("func (c *%s) %s(%s) error {", receiver, method, params),
	}
	lines = append(lines, body...)
	lines = append(lines, "}", "")
	return strings.Join(lines, "\n")
}

func buildRequestContent(baseName string) string {
	return strings.Join([]string{
		"package requests",
		"",
		fmt.Sprintf("// List%sRequest 列表查询请求。", baseName),
		fmt.Sprintf("type List%sRequest struct {", baseName),
		"\tPage int `query:\"page\" binding:\"omitempty,gte=1\"`",
		"\tSize int `query:\"size\" binding:\"omitempty,gte=1,lte=100\"`",
		"}",
		"",
		fmt.Sprintf("// Create%sRequest 创建请求。", baseName),
		fmt.Sprintf("type Create%sRequest struct {", baseName),
		"\t// TODO: 添加字段",
		"\t// Name string `json:\"name\" binding:\"required,min=2,max=50\"`",
		"}",
		"",
		fmt.Sprintf("// Update%sRequest 更新请求。", baseName),
		fmt.Sprintf("type Update%sRequest struct {", baseName),
		"\tID string `uri:\"id\" binding:\"required\"`",
		"\t// TODO: 添加字段",
		"}",
		"",
	}, "\n")
}
