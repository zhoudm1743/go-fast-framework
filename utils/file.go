package utils

import (
	"errors"
	"go/build"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gabriel-vasile/mimetype"
)

var FileUtil = fileUtil{}

type fileUtil struct{}

func (r fileUtil) Ignore(fn func() error) {
	_ = fn()
}

type FileOption func(*fileOption)

type fileOption struct {
	mode   os.FileMode
	append bool
}

// WithAppend sets the append mode for FilePutContents
func WithAppend() FileOption {
	return func(opts *fileOption) {
		opts.append = true
	}
}

// WithMode sets the file mode for FilePutContents
func WithMode(mode os.FileMode) FileOption {
	return func(opts *fileOption) {
		opts.mode = mode
	}
}

// DEPRECATED: Use Contains instead
func (r fileUtil) Contain(file string, search string) bool {
	return StringUtil.Contains(file, search)
}

// Create a file with the given content
// Deprecated: Use PutContent instead
func (r fileUtil) Create(file string, content string) error {
	if err := os.MkdirAll(filepath.Dir(file), os.ModePerm); err != nil {
		return err
	}

	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer r.Ignore(f.Close)

	if _, err = f.WriteString(content); err != nil {
		return err
	}

	return nil
}

func (r fileUtil) Exists(file string) bool {
	_, err := os.Stat(file)
	return err == nil
}

func (r fileUtil) Extension(file string, originalWhenUnknown ...bool) (string, error) {
	getOriginal := false
	if len(originalWhenUnknown) > 0 {
		getOriginal = originalWhenUnknown[0]
	}

	mtype, err := mimetype.DetectFile(file)
	if err != nil && !getOriginal {
		// 如果检测失败，尝试从文件名获取扩展名
		ext := filepath.Ext(file)
		if ext != "" {
			return strings.TrimPrefix(ext, "."), nil
		}
		return "", err
	}

	if mtype != nil && mtype.Extension() != "" {
		return strings.TrimPrefix(mtype.Extension(), "."), nil
	}

	// 如果 MIME 类型无法确定扩展名，从文件名获取
	ext := filepath.Ext(file)
	if ext != "" {
		return strings.TrimPrefix(ext, "."), nil
	}

	return "", errors.New("unknown file extension")
}

func (r fileUtil) GetContent(file string) (string, error) {
	// Read the entire file
	data, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}

	return StringUtil.UnsafeString(data), nil
}

func (r fileUtil) GetPackageContent(pkgName, file string) (string, error) {
	pkg, err := build.Import(pkgName, "", build.FindOnly)
	if err != nil {
		return "", err
	}

	paths := strings.Split(file, "/")
	paths = append([]string{pkg.Dir}, paths...)

	return r.GetContent(filepath.Join(paths...))
}

func (r fileUtil) LastModified(file, timezone string) (int64, error) {
	fileInfo, err := os.Stat(file)
	if err != nil {
		return 0, err
	}

	l, err := time.LoadLocation(timezone)
	if err != nil {
		return 0, err
	}

	return fileInfo.ModTime().In(l).UnixMilli(), nil
}

func (r fileUtil) MimeType(file string) (string, error) {
	mtype, err := mimetype.DetectFile(file)
	if err != nil {
		return "", err
	}

	return mtype.String(), nil
}

func (r fileUtil) PutContent(file string, content string, options ...FileOption) error {
	// Default options
	opts := &fileOption{
		mode:   os.ModePerm,
		append: false,
	}

	// Apply options
	for _, option := range options {
		option(opts)
	}

	// Create the directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(file), opts.mode); err != nil {
		return err
	}

	// Open file with appropriate flags
	flag := os.O_CREATE | os.O_WRONLY
	if opts.append {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}

	// Open the file
	f, err := os.OpenFile(file, flag, opts.mode)
	if err != nil {
		return err
	}
	defer r.Ignore(f.Close)

	// Write the content
	if _, err = f.WriteString(content); err != nil {
		return err
	}

	return nil
}

func (r fileUtil) Remove(file string) error {
	_, err := os.Stat(file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return err
	}

	return os.RemoveAll(file)
}

func (r fileUtil) Size(file string) (int64, error) {
	fileInfo, err := os.Open(file)
	if err != nil {
		return 0, err
	}
	defer r.Ignore(fileInfo.Close)

	fi, err := fileInfo.Stat()
	if err != nil {
		return 0, err
	}

	return fi.Size(), nil
}
