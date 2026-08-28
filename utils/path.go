package utils

import (
	"os"
	"path/filepath"
	"runtime"
)

// PathUtil 路径工具集（链式）。
var PathUtil = pathUtil{}

type pathUtil struct{}

type Path struct {
	value string
}

func (r pathUtil) Of(p string) *Path {
	return &Path{value: p}
}

func (p *Path) Join(elem ...string) *Path {
	all := append([]string{p.value}, elem...)
	p.value = filepath.Join(all...)
	return p
}

func (p *Path) Dir() *Path {
	p.value = filepath.Dir(p.value)
	return p
}

func (p *Path) Abs() *Path {
	if abs, err := filepath.Abs(p.value); err == nil {
		p.value = abs
	}
	return p
}

func (p *Path) ToSlash() *Path {
	p.value = filepath.ToSlash(p.value)
	return p
}

func (p *Path) String() string {
	return p.value
}

func (p *Path) Base() string {
	return filepath.Base(p.value)
}

func (p *Path) Ext() string {
	return filepath.Ext(p.value)
}

func (p *Path) IsAbs() bool {
	return filepath.IsAbs(p.value)
}

func (p *Path) Exists() bool {
	return FileUtil.Exists(p.value)
}

// OsUtil 系统工具集。
var OsUtil = osUtil{}

type osUtil struct{}

func (r osUtil) IsWindows() bool { return runtime.GOOS == "windows" }
func (r osUtil) IsLinux() bool   { return runtime.GOOS == "linux" }
func (r osUtil) IsDarwin() bool  { return runtime.GOOS == "darwin" }

func (r osUtil) HomeDir() (string, error) { return os.UserHomeDir() }
func (r osUtil) WorkDir() (string, error) { return os.Getwd() }
func (r osUtil) TempDir() string          { return os.TempDir() }
func (r osUtil) Hostname() (string, error) { return os.Hostname() }

func (r osUtil) Env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
