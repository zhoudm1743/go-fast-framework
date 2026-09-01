package mock

import (
	"context"
	"sync"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// MockStorage 实现 contracts.Storage 与 contracts.StorageDriver，用于测试。
type MockStorage struct {
	mu sync.Mutex

	DiskFunc             func(disk string) contracts.StorageDriver
	PutFunc              func(file, content string) error
	PutFileFunc          func(path string, source contracts.File) (string, error)
	PutFileAsFunc        func(path string, source contracts.File, name string) (string, error)
	GetFunc              func(file string) (string, error)
	GetBytesFunc         func(file string) ([]byte, error)
	ExistsFunc           func(file string) bool
	MissingFunc          func(file string) bool
	UrlFunc              func(file string) string
	TemporaryUrlFunc     func(file string, t int64) (string, error)
	CopyFunc             func(oldFile, newFile string) error
	MoveFunc             func(oldFile, newFile string) error
	DeleteFunc           func(files ...string) error
	SizeFunc             func(file string) (int64, error)
	LastModifiedFunc     func(file string) (int64, error)
	MimeTypeFunc         func(file string) (string, error)
	PathFunc             func(file string) string
	MakeDirectoryFunc    func(directory string) error
	DeleteDirectoryFunc  func(directory string) error
	FilesFunc            func(path string) ([]string, error)
	AllFilesFunc         func(path string) ([]string, error)
	DirectoriesFunc      func(path string) ([]string, error)
	AllDirectoriesFunc   func(path string) ([]string, error)
	WithContextFunc      func(ctx context.Context) contracts.StorageDriver

	// Files 模拟存储的文件内容。
	FilesData map[string][]byte
}

// NewMockStorage 创建 MockStorage。
func NewMockStorage() *MockStorage {
	return &MockStorage{
		FilesData: make(map[string][]byte),
	}
}

func (m *MockStorage) Disk(disk string) contracts.StorageDriver {
	if m.DiskFunc != nil {
		return m.DiskFunc(disk)
	}
	return m
}

func (m *MockStorage) Put(file, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.PutFunc != nil {
		return m.PutFunc(file, content)
	}
	m.FilesData[file] = []byte(content)
	return nil
}

func (m *MockStorage) PutFile(path string, source contracts.File) (string, error) {
	if m.PutFileFunc != nil {
		return m.PutFileFunc(path, source)
	}
	return path, nil
}

func (m *MockStorage) PutFileAs(path string, source contracts.File, name string) (string, error) {
	if m.PutFileAsFunc != nil {
		return m.PutFileAsFunc(path, source, name)
	}
	return path + "/" + name, nil
}

func (m *MockStorage) Get(file string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.GetFunc != nil {
		return m.GetFunc(file)
	}
	if b, ok := m.FilesData[file]; ok {
		return string(b), nil
	}
	return "", nil
}

func (m *MockStorage) GetBytes(file string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.GetBytesFunc != nil {
		return m.GetBytesFunc(file)
	}
	return m.FilesData[file], nil
}

func (m *MockStorage) Exists(file string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ExistsFunc != nil {
		return m.ExistsFunc(file)
	}
	_, ok := m.FilesData[file]
	return ok
}

func (m *MockStorage) Missing(file string) bool {
	if m.MissingFunc != nil {
		return m.MissingFunc(file)
	}
	return !m.Exists(file)
}

func (m *MockStorage) Url(file string) string {
	if m.UrlFunc != nil {
		return m.UrlFunc(file)
	}
	return "/storage/" + file
}

func (m *MockStorage) TemporaryUrl(file string, t int64) (string, error) {
	if m.TemporaryUrlFunc != nil {
		return m.TemporaryUrlFunc(file, t)
	}
	return m.Url(file), nil
}

func (m *MockStorage) Copy(oldFile, newFile string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.CopyFunc != nil {
		return m.CopyFunc(oldFile, newFile)
	}
	if b, ok := m.FilesData[oldFile]; ok {
		m.FilesData[newFile] = b
	}
	return nil
}

func (m *MockStorage) Move(oldFile, newFile string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.MoveFunc != nil {
		return m.MoveFunc(oldFile, newFile)
	}
	if b, ok := m.FilesData[oldFile]; ok {
		m.FilesData[newFile] = b
		delete(m.FilesData, oldFile)
	}
	return nil
}

func (m *MockStorage) Delete(files ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.DeleteFunc != nil {
		return m.DeleteFunc(files...)
	}
	for _, f := range files {
		delete(m.FilesData, f)
	}
	return nil
}

func (m *MockStorage) Size(file string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.SizeFunc != nil {
		return m.SizeFunc(file)
	}
	return int64(len(m.FilesData[file])), nil
}

func (m *MockStorage) LastModified(file string) (int64, error) {
	if m.LastModifiedFunc != nil {
		return m.LastModifiedFunc(file)
	}
	return 0, nil
}

func (m *MockStorage) MimeType(file string) (string, error) {
	if m.MimeTypeFunc != nil {
		return m.MimeTypeFunc(file)
	}
	return "application/octet-stream", nil
}

func (m *MockStorage) Path(file string) string {
	if m.PathFunc != nil {
		return m.PathFunc(file)
	}
	return file
}

func (m *MockStorage) MakeDirectory(directory string) error {
	if m.MakeDirectoryFunc != nil {
		return m.MakeDirectoryFunc(directory)
	}
	return nil
}

func (m *MockStorage) DeleteDirectory(directory string) error {
	if m.DeleteDirectoryFunc != nil {
		return m.DeleteDirectoryFunc(directory)
	}
	return nil
}

func (m *MockStorage) Files(path string) ([]string, error) {
	if m.FilesFunc != nil {
		return m.FilesFunc(path)
	}
	return []string{}, nil
}

func (m *MockStorage) AllFiles(path string) ([]string, error) {
	if m.AllFilesFunc != nil {
		return m.AllFilesFunc(path)
	}
	return []string{}, nil
}

func (m *MockStorage) Directories(path string) ([]string, error) {
	if m.DirectoriesFunc != nil {
		return m.DirectoriesFunc(path)
	}
	return []string{}, nil
}

func (m *MockStorage) AllDirectories(path string) ([]string, error) {
	if m.AllDirectoriesFunc != nil {
		return m.AllDirectoriesFunc(path)
	}
	return []string{}, nil
}

func (m *MockStorage) WithContext(ctx context.Context) contracts.StorageDriver {
	if m.WithContextFunc != nil {
		return m.WithContextFunc(ctx)
	}
	return m
}
