// Package view 提供基于 html/template 的 HTML 模板渲染引擎，
// 实现 contracts.ViewEngine 接口。
//
// 功能概述：
//   - 递归加载指定目录下所有模板文件（按扩展名过滤）
//   - 模板名称为相对目录的路径，路径分隔符统一为 "/"（跨平台）
//   - 支持自定义模板函数（AddFunc / AddFuncMap）
//   - 惰性加载：首次 Render 时自动解析模板
//   - 开发模式：每次 Render 前重新加载（WithReload）
//   - 线程安全
//
// 基本用法：
//
//	engine := view.New("resources/views",
//	    view.WithExtension(".html"),
//	    view.WithFuncMap(template.FuncMap{
//	        "upper": strings.ToUpper,
//	    }),
//	    view.WithReload(true), // 开发模式
//	)
//
// 在控制器中使用：
//
//	func Index(ctx contracts.Context) error {
//	    return ctx.Response().View("home/index.html", gin.H{"title": "首页"})
//	}
package view

import (
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// Engine 是基于 html/template 的模板渲染引擎。
//
// 每个页面文件拥有独立的 template.Template 集合（包含 layout/ 目录下的所有布局
// 文件 + 该页面文件本身），从而避免不同页面的同名 {{define "content"}} 互相覆盖。
type Engine struct {
	mu      sync.RWMutex
	pages   map[string]*template.Template // 每个页面独立的模板集合
	dir     string                        // 模板根目录（磁盘路径 或 fs.FS 内的根路径）
	ext     string                        // 过滤的文件扩展名，例如 ".html"
	funcMap template.FuncMap              // 自定义模板函数
	loaded  bool                          // 模板是否已加载
	reload  bool                          // 是否每次 Render 都重新加载（开发模式）
	fsys    fs.FS                         // 非 nil 时从该 FS 加载（用于 go:embed）
}

// Option 是 Engine 配置选项函数。
type Option func(*Engine)

// WithExtension 设置模板文件扩展名过滤器，默认为 ".html"。
// 传入空字符串则加载目录下所有文件。
func WithExtension(ext string) Option {
	return func(e *Engine) { e.ext = ext }
}

// WithFuncMap 向引擎注册自定义模板函数（合并到现有 FuncMap）。
func WithFuncMap(fm template.FuncMap) Option {
	return func(e *Engine) {
		for k, v := range fm {
			e.funcMap[k] = v
		}
	}
}

// WithReload 启用每次渲染前重新加载模板（开发模式）。
// 生产环境请设置为 false（默认）以获得最佳性能。
func WithReload(reload bool) Option {
	return func(e *Engine) { e.reload = reload }
}

// WithFS 让引擎从 fsys（如 embed.FS）中加载模板，而不是从操作系统磁盘。
// root 为 fsys 内的子路径，例如 "resources/views"；传 "." 表示 FS 根目录。
//
// 典型用法（go:embed）：
//
//	//go:embed resources/views
//	var viewFS embed.FS
//
//	sub, _ := fs.Sub(viewFS, "resources/views")
//	engine := view.New(".", view.WithFS(sub))
func WithFS(fsys fs.FS, root string) Option {
	return func(e *Engine) {
		e.fsys = fsys
		e.dir = root
	}
}

// New 创建一个从 dir 目录加载模板的 Engine。
// 模板采用惰性加载，首次 Render 时才解析。
func New(dir string, opts ...Option) *Engine {
	e := &Engine{
		dir:     dir,
		ext:     ".html",
		funcMap: make(template.FuncMap),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// AddFunc 注册单个自定义模板函数，并使缓存的模板失效以触发重新解析。
// 实现 contracts.ViewEngine。
func (e *Engine) AddFunc(name string, fn any) contracts.ViewEngine {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.funcMap[name] = fn
	e.loaded = false
	return e
}

// AddFuncMap 批量注册自定义模板函数，并使缓存失效。
// 实现 contracts.ViewEngine。
func (e *Engine) AddFuncMap(fm template.FuncMap) contracts.ViewEngine {
	e.mu.Lock()
	defer e.mu.Unlock()
	for k, v := range fm {
		e.funcMap[k] = v
	}
	e.loaded = false
	return e
}

// Load 从磁盘强制（重新）加载所有模板（线程安全）。
// 实现 contracts.ViewEngine。
func (e *Engine) Load() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.load()
}

// load 必须在持有写锁的情况下调用。
//
// 实现策略：
//  1. 收集所有模板文件的内容（rel → content）。
//  2. 将 layout/ 目录内的文件识别为"公共布局"。
//  3. 对 其余每个 页面文件，创建独立的 *template.Template：
//     先解析所有布局文件，再解析该页面文件。
//     这样各页面的 {{define "content"}} 互不干扰。
func (e *Engine) load() error {
	// Step 1: 收集所有文件内容
	contents := make(map[string]string) // rel path → content
	if e.fsys != nil {
		err := fs.WalkDir(e.fsys, e.dir, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() {
				return walkErr
			}
			if e.ext != "" && !strings.HasSuffix(path, e.ext) {
				return nil
			}
			rel := path
			prefix := e.dir
			if prefix != "." && prefix != "" {
				rel = strings.TrimPrefix(path, prefix+"/")
			}
			b, err := fs.ReadFile(e.fsys, path)
			if err != nil {
				return err
			}
			contents[rel] = string(b)
			return nil
		})
		if err != nil {
			return err
		}
	} else {
		err := filepath.WalkDir(e.dir, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() {
				return walkErr
			}
			if e.ext != "" && filepath.Ext(path) != e.ext {
				return nil
			}
			rel, err := filepath.Rel(e.dir, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			contents[rel] = string(b)
			return nil
		})
		if err != nil {
			return err
		}
	}

	// Step 2: 将 layout/ 目录内的文件识别为公共布局
	var layoutFiles []string
	var pageFiles []string
	for rel := range contents {
		if strings.HasPrefix(rel, "layout/") {
			layoutFiles = append(layoutFiles, rel)
		} else {
			pageFiles = append(pageFiles, rel)
		}
	}

	// Step 3: 为每个页面文件建立独立的 template.Template 集合
	pages := make(map[string]*template.Template, len(pageFiles))
	for _, name := range pageFiles {
		t := template.New("").Funcs(e.funcMap)
		// 先解析所有布局文件
		for _, lf := range layoutFiles {
			if _, err := t.New(lf).Parse(contents[lf]); err != nil {
				return fmt.Errorf("view: parse layout %q: %w", lf, err)
			}
		}
		// 再解析该页面文件（其 {{define}} 块只注册在本 set 中）
		if _, err := t.New(name).Parse(contents[name]); err != nil {
			return fmt.Errorf("view: parse template %q: %w", name, err)
		}
		pages[name] = t
	}

	e.pages = pages
	e.loaded = true
	return nil
}

// Render 将指定名称的模板与 data 合并后写入 w。
// 实现 contracts.ViewEngine。
//
// name 为相对于模板目录的路径，例如 "home/index.html"。
// 首次调用时自动触发模板加载；开发模式（WithReload）下每次都会重新加载。
func (e *Engine) Render(w io.Writer, name string, data any) error {
	if e.reload {
		// 开发模式：每次渲染前重新加载
		e.mu.Lock()
		if err := e.load(); err != nil {
			e.mu.Unlock()
			return err
		}
		t := e.pages[name]
		e.mu.Unlock()
		if t == nil {
			return fmt.Errorf("view: template %q not found", name)
		}
		return t.ExecuteTemplate(w, name, data)
	}

	// 惰性加载：首次 Render 时才解析
	e.mu.RLock()
	loaded := e.loaded
	e.mu.RUnlock()

	if !loaded {
		e.mu.Lock()
		// double-check：防止并发时重复加载
		if !e.loaded {
			if err := e.load(); err != nil {
				e.mu.Unlock()
				return err
			}
		}
		e.mu.Unlock()
	}

	e.mu.RLock()
	t := e.pages[name]
	e.mu.RUnlock()

	if t == nil {
		return fmt.Errorf("view: template %q not found", name)
	}
	return t.ExecuteTemplate(w, name, data)
}
