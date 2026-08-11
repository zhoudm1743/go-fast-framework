package filesystem

import (
	"context"
	"fmt"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

type storage struct {
	drivers     map[string]contracts.StorageDriver
	defaultDisk string
}

func NewStorage(cfg contracts.Config) (contracts.StorageDriver, error) {
	defaultDisk := cfg.GetString("filesystem.default", "local")

	s := &storage{
		drivers:     make(map[string]contracts.StorageDriver),
		defaultDisk: defaultDisk,
	}

	disks := cfg.Get("filesystem.disks")
	if disksMap, ok := disks.(map[string]any); ok {
		for name, diskCfgRaw := range disksMap {
			diskCfg, ok := diskCfgRaw.(map[string]any)
			if !ok {
				continue
			}
			driver, _ := diskCfg["driver"].(string)
			switch driver {
			case "local", "":
				root, _ := diskCfg["root"].(string)
				url, _ := diskCfg["url"].(string)
				if root == "" {
					root = "storage/app"
				}
				if url == "" {
					url = "/storage"
				}
				d, err := newLocalDriver(root, url)
				if err != nil {
					return nil, fmt.Errorf("[GoFast] storage: init disk %q failed: %w", name, err)
				}
				s.drivers[name] = d
			case "oss":
				d, err := newOssDriver(name, diskCfg)
				if err != nil {
					return nil, fmt.Errorf("[GoFast] storage: init disk %q failed: %w", name, err)
				}
				s.drivers[name] = d
			case "cos":
				d, err := newCosDriver(name, diskCfg)
				if err != nil {
					return nil, fmt.Errorf("[GoFast] storage: init disk %q failed: %w", name, err)
				}
				s.drivers[name] = d
			case "minio":
				d, err := newMinioDriver(name, diskCfg)
				if err != nil {
					return nil, fmt.Errorf("[GoFast] storage: init disk %q failed: %w", name, err)
				}
				s.drivers[name] = d
			case "s3":
				d, err := newS3Driver(name, diskCfg)
				if err != nil {
					return nil, fmt.Errorf("[GoFast] storage: init disk %q failed: %w", name, err)
				}
				s.drivers[name] = d
			}
		}
	}

	if _, ok := s.drivers["local"]; !ok {
		d, err := newLocalDriver("storage/app", "/storage")
		if err != nil {
			return nil, err
		}
		s.drivers["local"] = d
	}

	return s, nil
}

func (s *storage) defaultDriver() contracts.StorageDriver {
	if d, ok := s.drivers[s.defaultDisk]; ok {
		return d
	}
	return s.drivers["local"]
}

func (s *storage) Disk(disk string) contracts.StorageDriver {
	if d, ok := s.drivers[disk]; ok {
		return d
	}
	return s.drivers["local"]
}

func (s *storage) Put(file, content string) error { return s.defaultDriver().Put(file, content) }
func (s *storage) PutFile(p string, src contracts.File) (string, error) {
	return s.defaultDriver().PutFile(p, src)
}
func (s *storage) PutFileAs(p string, src contracts.File, n string) (string, error) {
	return s.defaultDriver().PutFileAs(p, src, n)
}
func (s *storage) Get(file string) (string, error)      { return s.defaultDriver().Get(file) }
func (s *storage) GetBytes(file string) ([]byte, error) { return s.defaultDriver().GetBytes(file) }
func (s *storage) Exists(file string) bool              { return s.defaultDriver().Exists(file) }
func (s *storage) Missing(file string) bool             { return s.defaultDriver().Missing(file) }
func (s *storage) Url(file string) string               { return s.defaultDriver().Url(file) }
func (s *storage) TemporaryUrl(file string, t int64) (string, error) {
	return s.defaultDriver().TemporaryUrl(file, t)
}
func (s *storage) Copy(o, n string) error          { return s.defaultDriver().Copy(o, n) }
func (s *storage) Move(o, n string) error          { return s.defaultDriver().Move(o, n) }
func (s *storage) Delete(files ...string) error    { return s.defaultDriver().Delete(files...) }
func (s *storage) Size(file string) (int64, error) { return s.defaultDriver().Size(file) }
func (s *storage) LastModified(file string) (int64, error) {
	return s.defaultDriver().LastModified(file)
}
func (s *storage) MimeType(file string) (string, error)   { return s.defaultDriver().MimeType(file) }
func (s *storage) Path(file string) string                { return s.defaultDriver().Path(file) }
func (s *storage) MakeDirectory(dir string) error         { return s.defaultDriver().MakeDirectory(dir) }
func (s *storage) DeleteDirectory(dir string) error       { return s.defaultDriver().DeleteDirectory(dir) }
func (s *storage) Files(path string) ([]string, error)    { return s.defaultDriver().Files(path) }
func (s *storage) AllFiles(path string) ([]string, error) { return s.defaultDriver().AllFiles(path) }
func (s *storage) Directories(path string) ([]string, error) {
	return s.defaultDriver().Directories(path)
}
func (s *storage) AllDirectories(path string) ([]string, error) {
	return s.defaultDriver().AllDirectories(path)
}
func (s *storage) WithContext(ctx context.Context) contracts.StorageDriver {
	return s.defaultDriver().WithContext(ctx)
}
