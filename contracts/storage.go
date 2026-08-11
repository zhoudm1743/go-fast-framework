package contracts

import "context"

// Storage 文件存储服务契约，嵌入 StorageDriver 接口并支持多磁盘切换。
type Storage interface {
	StorageDriver
	// Disk 获取指定磁盘的驱动实例。
	Disk(disk string) StorageDriver
}

// Driver 文件系统驱动契约。
type StorageDriver interface {
	// Put 写入文件内容。
	Put(file, content string) error
	// PutFile 上传文件到指定路径。
	PutFile(path string, source File) (string, error)
	// PutFileAs 上传文件到指定路径并重命名。
	PutFileAs(path string, source File, name string) (string, error)
	// Get 读取文件内容。
	Get(file string) (string, error)
	// GetBytes 以字节数组读取文件。
	GetBytes(file string) ([]byte, error)
	// Exists 判断文件是否存在。
	Exists(file string) bool
	// Missing 判断文件是否不存在。
	Missing(file string) bool
	// Url 获取文件的访问 URL。
	Url(file string) string
	// TemporaryUrl 获取临时访问 URL。
	TemporaryUrl(file string, time int64) (string, error)
	// Copy 复制文件。
	Copy(oldFile, newFile string) error
	// Move 移动文件。
	Move(oldFile, newFile string) error
	// Delete 删除文件。
	Delete(file ...string) error
	// Size 获取文件大小。
	Size(file string) (int64, error)
	// LastModified 获取最后修改时间。
	LastModified(file string) (int64, error)
	// MimeType 获取文件 MIME 类型。
	MimeType(file string) (string, error)
	// Path 获取文件完整路径。
	Path(file string) string
	// MakeDirectory 创建目录。
	MakeDirectory(directory string) error
	// DeleteDirectory 删除目录（递归）。
	DeleteDirectory(directory string) error
	// Files 获取目录下所有文件。
	Files(path string) ([]string, error)
	// AllFiles 递归获取目录下所有文件。
	AllFiles(path string) ([]string, error)
	// Directories 获取目录下所有子目录。
	Directories(path string) ([]string, error)
	// AllDirectories 递归获取所有子目录。
	AllDirectories(path string) ([]string, error)
	// WithContext 设置上下文。
	WithContext(ctx context.Context) StorageDriver
}

// File 上传文件抽象。
type File interface {
	Disk(disk string) File
	File() string
	Store(path string) (string, error)
	StoreAs(path string, name string) (string, error)
	GetClientOriginalName() string
	GetClientOriginalExtension() string
	HashName(path ...string) string
	Extension() (string, error)
	MimeType() (string, error)
	Size() (int64, error)
	LastModified() (int64, error)
}
