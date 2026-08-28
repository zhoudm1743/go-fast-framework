package fast

import (
	"os"
	"path/filepath"
	"testing"
)

func TestToSnakeCaseGolden(t *testing.T) {
	cases := map[string]string{
		"UserProfile":    "user_profile",
		"HTTPController": "http_controller",
		"XMLParser":      "xml_parser",
		"user":           "user",
		"PostController": "post_controller",
	}
	for in, want := range cases {
		if got := toSnakeCase(in); got != want {
			t.Fatalf("toSnakeCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestToPascalCaseGolden(t *testing.T) {
	cases := map[string]string{
		"user":           "User",
		"UserProfile":    "UserProfile",
		"postController": "PostController",
		"":               "",
	}
	for in, want := range cases {
		if got := toPascalCase(in); got != want {
			t.Fatalf("toPascalCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWriteGeneratedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "new.go")

	if err := writeGeneratedFile(path, "hello"); err != nil {
		t.Fatalf("首次写入失败: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("内容不匹配: %q", data)
	}

	if err := writeGeneratedFile(path, "other"); err == nil {
		t.Fatal("文件已存在时应返回错误")
	}
}
