package filesystem

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/id"
)

const cloudMaxKeys = 1000

// validPath 规范化云存储路径前缀（以 "/" 结尾，去掉多余前缀）。
func validPath(path string) string {
	realPath := strings.TrimPrefix(path, "./")
	realPath = strings.TrimPrefix(realPath, "/")
	realPath = strings.TrimPrefix(realPath, ".")
	if realPath != "" && !strings.HasSuffix(realPath, "/") {
		realPath += "/"
	}
	return realPath
}

// cloudJoinPath 拼接云存储对象 key（dir + "/" + name，均去掉多余斜杠）。
func cloudJoinPath(dir, name string) string {
	dir = strings.TrimSuffix(strings.TrimPrefix(dir, "/"), "/")
	name = filepath.ToSlash(strings.TrimPrefix(filepath.Base(name), "/"))
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

// cloudFileAs 计算上传后对象的 key（path/name.ext）。
func cloudFileAs(filePath string, source contracts.File, name string) (string, error) {
	ext := filepath.Ext(name)
	baseName := strings.TrimSuffix(filepath.Base(name), ext)
	if ext == "" {
		sourceExt, err := source.Extension()
		if err != nil {
			return "", err
		}
		if sourceExt != "" {
			name = baseName + "." + sourceExt
		} else {
			name = baseName
		}
	}
	return cloudJoinPath(filePath, name), nil
}

// cloudFile 生成随机 UUID 文件名并计算对象 key。
func cloudFile(filePath string, source contracts.File) (string, error) {
	ext, err := source.Extension()
	if err != nil {
		return "", err
	}
	name := id.New()
	if ext != "" {
		name = name + "." + ext
	}
	return cloudJoinPath(filePath, name), nil
}

// getString 从 map[string]any 安全取 string。
func getString(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

// getBool 从 map[string]any 安全取 bool。
func getBool(m map[string]any, key string) bool {
	v, _ := m[key].(bool)
	return v
}

// openFileForUpload 打开文件用于上传（调用方负责关闭）。
func openFileForUpload(path string) (*os.File, error) {
	return os.Open(path)
}

// fileSize 获取文件字节大小。
func fileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}
