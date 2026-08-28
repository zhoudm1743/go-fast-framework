package fast

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zhoudm1743/go-fast-framework/utils"
)

// ─── 模块名读取 ───────────────────────────────────────────────────────────────

// readGoMod 从当前工作目录的 go.mod 读取模块名。
func readGoMod() string {
	wd, _ := os.Getwd()
	f, err := os.Open(filepath.Join(wd, "go.mod"))
	if err != nil {
		return "github.com/your-module"
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return "github.com/your-module"
}

// ─── 字符串工具 ───────────────────────────────────────────────────────────────

// toSnakeCase 将 PascalCase / camelCase 转换为 snake_case。
// UserProfile → user_profile
func toSnakeCase(s string) string {
	return utils.StringUtil.Of(s).Snake().String()
}

// toPascalCase 确保首字母大写（PascalCase）。
func toPascalCase(s string) string {
	return utils.StringUtil.Of(s).UcFirst().String()
}

// toRoutePath 将 PascalCase 类名转换为 URL 路径（小写复数）。
// UserProfile → /user-profiles
func toRoutePath(name string) string {
	snake := toSnakeCase(name)
	parts := strings.Split(snake, "_")
	last := parts[len(parts)-1]
	// 简单复数化：以 s/x/ch/sh 结尾加 es，否则加 s
	switch {
	case strings.HasSuffix(last, "s") ||
		strings.HasSuffix(last, "x") ||
		strings.HasSuffix(last, "ch") ||
		strings.HasSuffix(last, "sh"):
		last += "es"
	default:
		last += "s"
	}
	parts[len(parts)-1] = last
	return "/" + strings.Join(parts, "-")
}

// stripSuffix 如果 s 以 suffix 结尾（不区分大小写），去掉该后缀。
// "PostController" → "Post"（suffix="Controller"）
func stripSuffix(s, suffix string) string {
	if strings.HasSuffix(strings.ToLower(s), strings.ToLower(suffix)) {
		return s[:len(s)-len(suffix)]
	}
	return s
}

// ─── 文件写入 ──────────────────────────────────────────────────────────────────

// writeGeneratedFile 创建目录并写入文件。文件已存在时返回错误。
func writeGeneratedFile(path, content string) error {
	if err := utils.FileUtil.Mkdir(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	if utils.FileUtil.Exists(path) {
		return fmt.Errorf("文件已存在: %s", path)
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// relPath 返回相对于当前工作目录的路径（用于打印）。
func relPath(path string) string {
	wd, _ := os.Getwd()
	rel, err := filepath.Rel(wd, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}
