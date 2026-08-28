package utils

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
)

// ZipUtil 压缩工具集。
var ZipUtil = zipUtil{}

type zipUtil struct{}

func (r zipUtil) Zip(files map[string]string, dest string) error {
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer FileUtil.Ignore(out.Close)
	w := zip.NewWriter(out)
	defer w.Close()
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			return err
		}
		if _, err := f.Write([]byte(content)); err != nil {
			return err
		}
	}
	return w.Close()
}

func (r zipUtil) Unzip(src, dest string) error {
	rdr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer rdr.Close()
	for _, f := range rdr.File {
		path := filepath.Join(dest, f.Name)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(path, f.Mode()); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(path)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (r zipUtil) ZipDir(src, dest string) error {
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer FileUtil.Ignore(out.Close)
	w := zip.NewWriter(out)
	defer w.Close()
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		f, err := w.Create(rel)
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(f, in)
		return err
	})
}
