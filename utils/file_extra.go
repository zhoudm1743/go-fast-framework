package utils

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func (r fileUtil) IsFile(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

func (r fileUtil) IsDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

func (r fileUtil) Mkdir(path string, perm ...os.FileMode) error {
	mode := os.ModePerm
	if len(perm) > 0 {
		mode = perm[0]
	}
	return os.MkdirAll(path, mode)
}

func (r fileUtil) Copy(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer r.Ignore(in.Close)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer r.Ignore(out.Close)
	_, err = io.Copy(out, in)
	return err
}

func (r fileUtil) CopyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return r.Copy(path, target)
	})
}

func (r fileUtil) Move(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.Rename(src, dst)
}

func (r fileUtil) ReadLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer r.Ignore(f.Close)
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines, sc.Err()
}

func (r fileUtil) WriteLines(path string, lines []string, options ...FileOption) error {
	return r.PutContent(path, strings.Join(lines, "\n"), options...)
}

func (r fileUtil) ContainsContent(path, search string) (bool, error) {
	content, err := r.GetContent(path)
	if err != nil {
		return false, err
	}
	return strings.Contains(content, search), nil
}

func (r fileUtil) NameWithoutExt(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

func (r fileUtil) Md5File(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return ToolsUtil.Md5(string(content)), nil
}

func (r fileUtil) Sha256File(path string) (string, error) {
	return HashUtil.Sha256File(path)
}
