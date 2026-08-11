package filesystem

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/id"
)

type localDriver struct {
	root string
	url  string
}

func newLocalDriver(root, url string) (*localDriver, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("[GoFast] storage local: invalid root %q: %w", root, err)
	}
	if err := os.MkdirAll(absRoot, 0755); err != nil {
		return nil, fmt.Errorf("[GoFast] storage local: create root failed: %w", err)
	}
	return &localDriver{root: absRoot, url: strings.TrimSuffix(url, "/")}, nil
}

func (d *localDriver) fullPath(file string) string {
	return filepath.Join(d.root, filepath.FromSlash(file))
}

func (d *localDriver) Put(file, content string) error {
	p := d.fullPath(file)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(content), 0644)
}

func (d *localDriver) PutFile(path string, source contracts.File) (string, error) {
	ext, err := source.Extension()
	if err != nil {
		return "", err
	}
	name := id.New()
	if ext != "" {
		name = name + "." + ext
	}
	return d.copyFromSource(path, name, source.File())
}

func (d *localDriver) PutFileAs(path string, source contracts.File, name string) (string, error) {
	return d.copyFromSource(path, name, source.File())
}

// copyFromSource 将 src 文件拷贝到 root/dir/name，返回相对路径 dir/name。
func (d *localDriver) copyFromSource(dir, name, src string) (string, error) {
	rel := filepath.ToSlash(filepath.Join(dir, name))
	dst := d.fullPath(rel)
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return "", err
	}
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return "", err
	}
	return rel, nil
}

func (d *localDriver) Get(file string) (string, error) {
	b, err := os.ReadFile(d.fullPath(file))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (d *localDriver) GetBytes(file string) ([]byte, error) {
	return os.ReadFile(d.fullPath(file))
}

func (d *localDriver) Exists(file string) bool {
	_, err := os.Stat(d.fullPath(file))
	return err == nil
}

func (d *localDriver) Missing(file string) bool {
	return !d.Exists(file)
}

func (d *localDriver) Url(file string) string {
	return d.url + "/" + strings.TrimPrefix(filepath.ToSlash(file), "/")
}

func (d *localDriver) TemporaryUrl(file string, _ int64) (string, error) {
	return d.Url(file), nil
}

func (d *localDriver) Copy(oldFile, newFile string) error {
	src := d.fullPath(oldFile)
	dst := d.fullPath(newFile)
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func (d *localDriver) Move(oldFile, newFile string) error {
	src := d.fullPath(oldFile)
	dst := d.fullPath(newFile)
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return os.Rename(src, dst)
}

func (d *localDriver) Delete(files ...string) error {
	for _, f := range files {
		if err := os.Remove(d.fullPath(f)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (d *localDriver) Size(file string) (int64, error) {
	info, err := os.Stat(d.fullPath(file))
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func (d *localDriver) LastModified(file string) (int64, error) {
	info, err := os.Stat(d.fullPath(file))
	if err != nil {
		return 0, err
	}
	return info.ModTime().Unix(), nil
}

func (d *localDriver) MimeType(file string) (string, error) {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(file), "."))
	if m, ok := mimeTypes[ext]; ok {
		return m, nil
	}
	return "application/octet-stream", nil
}

func (d *localDriver) Path(file string) string {
	return d.fullPath(file)
}

func (d *localDriver) MakeDirectory(directory string) error {
	return os.MkdirAll(d.fullPath(directory), 0755)
}

func (d *localDriver) DeleteDirectory(directory string) error {
	return os.RemoveAll(d.fullPath(directory))
}

func (d *localDriver) Files(path string) ([]string, error) {
	return listDir(d.fullPath(path), false, false)
}

func (d *localDriver) AllFiles(path string) ([]string, error) {
	return listDir(d.fullPath(path), false, true)
}

func (d *localDriver) Directories(path string) ([]string, error) {
	return listDir(d.fullPath(path), true, false)
}

func (d *localDriver) AllDirectories(path string) ([]string, error) {
	return listDir(d.fullPath(path), true, true)
}

func (d *localDriver) WithContext(_ context.Context) contracts.StorageDriver {
	return d
}

func listDir(root string, dirsOnly bool, recursive bool) ([]string, error) {
	var result []string
	if recursive {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if path == root {
				return nil
			}
			if dirsOnly && !info.IsDir() {
				return nil
			}
			if !dirsOnly && info.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			result = append(result, filepath.ToSlash(rel))
			return nil
		})
		return result, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if dirsOnly && !e.IsDir() {
			continue
		}
		if !dirsOnly && e.IsDir() {
			continue
		}
		result = append(result, e.Name())
	}
	return result, nil
}

var mimeTypes = map[string]string{
	"jpg": "image/jpeg", "jpeg": "image/jpeg", "png": "image/png",
	"gif": "image/gif", "webp": "image/webp", "svg": "image/svg+xml",
	"bmp": "image/bmp", "ico": "image/x-icon",
	"pdf":  "application/pdf",
	"doc":  "application/msword",
	"docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	"xls":  "application/vnd.ms-excel",
	"xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	"zip":  "application/zip", "rar": "application/x-rar-compressed",
	"mp4": "video/mp4", "mp3": "audio/mpeg", "wav": "audio/wav",
	"txt": "text/plain", "csv": "text/csv", "md": "text/markdown",
	"json": "application/json", "xml": "application/xml",
	"js": "application/javascript", "css": "text/css", "html": "text/html",
}
