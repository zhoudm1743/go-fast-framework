package filesystem

import (
	"io"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// UploadedFile 封装 HTTP multipart 上传文件，实现 contracts.File。
// 通过 Context.Files(key) 获取，可直接调用 Store / StoreAs 持久化到任意存储磁盘。
type UploadedFile struct {
	header  *multipart.FileHeader
	storage contracts.Storage
	disk    string

	tempOnce sync.Once
	tempPath string
}

// NewUploadedFile 创建上传文件包装器。
func NewUploadedFile(header *multipart.FileHeader, storage contracts.Storage) *UploadedFile {
	return &UploadedFile{header: header, storage: storage}
}

// Disk 切换目标存储磁盘，返回新的 File 实例（不影响原始实例）。
func (f *UploadedFile) Disk(disk string) contracts.File {
	cp := &UploadedFile{
		header:  f.header,
		storage: f.storage,
		disk:    disk,
	}
	return cp
}

// File 返回上传内容的本地临时文件路径。
// 首次调用时将 multipart 数据写入系统临时目录，后续调用直接复用该路径。
func (f *UploadedFile) File() string {
	f.tempOnce.Do(func() {
		src, err := f.header.Open()
		if err != nil {
			return
		}
		defer src.Close()
		tmp, err := os.CreateTemp("", "gofast-upload-*"+filepath.Ext(f.header.Filename))
		if err != nil {
			return
		}
		defer tmp.Close()
		if _, err = io.Copy(tmp, src); err != nil {
			return
		}
		f.tempPath = tmp.Name()
	})
	return f.tempPath
}

func (f *UploadedFile) driver() contracts.StorageDriver {
	if f.disk != "" {
		return f.storage.Disk(f.disk)
	}
	return f.storage
}

// Store 以随机 UUID 文件名将上传文件存入指定目录，返回可访问的 URL。
func (f *UploadedFile) Store(path string) (string, error) {
	d := f.driver()
	key, err := d.PutFile(path, f)
	if err != nil {
		return "", err
	}
	return d.Url(key), nil
}

// StoreAs 以指定文件名将上传文件存入指定目录，返回可访问的 URL。
func (f *UploadedFile) StoreAs(path string, name string) (string, error) {
	d := f.driver()
	key, err := d.PutFileAs(path, f, name)
	if err != nil {
		return "", err
	}
	return d.Url(key), nil
}

// GetClientOriginalName 返回客户端上传时的原始文件名。
func (f *UploadedFile) GetClientOriginalName() string {
	return f.header.Filename
}

// GetClientOriginalExtension 返回原始文件扩展名（不含点号）。
func (f *UploadedFile) GetClientOriginalExtension() string {
	return strings.TrimPrefix(filepath.Ext(f.header.Filename), ".")
}

// HashName 返回 UUID 随机文件名（含扩展名），可传入可选路径前缀。
func (f *UploadedFile) HashName(path ...string) string {
	name := uuid.New().String() + filepath.Ext(f.header.Filename)
	if len(path) > 0 && path[0] != "" {
		return strings.TrimSuffix(path[0], "/") + "/" + name
	}
	return name
}

// Extension 返回文件扩展名（不含点号）。
func (f *UploadedFile) Extension() (string, error) {
	return strings.TrimPrefix(filepath.Ext(f.header.Filename), "."), nil
}

// MimeType 返回 MIME 类型；优先使用上传时的 Content-Type，fallback 到扩展名推断。
func (f *UploadedFile) MimeType() (string, error) {
	ct := f.header.Header.Get("Content-Type")
	if ct != "" {
		mtype, _, err := mime.ParseMediaType(ct)
		if err == nil {
			return mtype, nil
		}
	}
	return mime.TypeByExtension(filepath.Ext(f.header.Filename)), nil
}

// Size 返回文件字节大小。
func (f *UploadedFile) Size() (int64, error) {
	return f.header.Size, nil
}

// LastModified 返回当前时间戳（上传文件无历史修改时间）。
func (f *UploadedFile) LastModified() (int64, error) {
	return time.Now().Unix(), nil
}
