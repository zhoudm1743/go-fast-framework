package filesystem

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/zhoudm1743/go-fast-framework/contracts"
)

type minioDriver struct {
	ctx      context.Context
	instance *minio.Client
	bucket   string
	diskName string
	url      string
}

func newMinioDriver(name string, cfg map[string]any) (*minioDriver, error) {
	key := getString(cfg, "key")
	secret := getString(cfg, "secret")
	bucket := getString(cfg, "bucket")
	diskURL := getString(cfg, "url")
	endpoint := getString(cfg, "endpoint")
	region := getString(cfg, "region")
	ssl := getBool(cfg, "ssl")

	if key == "" || secret == "" || bucket == "" || diskURL == "" || endpoint == "" {
		return nil, fmt.Errorf("[GoFast] minio disk %q: key/secret/bucket/url/endpoint are required", name)
	}

	// 去掉 endpoint 的协议前缀（minio-go 要求不带协议）
	endpoint = strings.TrimPrefix(endpoint, "https://")
	endpoint = strings.TrimPrefix(endpoint, "http://")

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(key, secret, ""),
		Secure: ssl,
		Region: region,
	})
	if err != nil {
		return nil, fmt.Errorf("[GoFast] minio disk %q: init client: %w", name, err)
	}

	return &minioDriver{
		ctx:      context.Background(),
		instance: client,
		bucket:   bucket,
		diskName: name,
		url:      strings.TrimSuffix(diskURL, "/"),
	}, nil
}

func (d *minioDriver) Put(file, content string) error {
	mtype := mimetype.Detect([]byte(content))
	reader := strings.NewReader(content)
	_, err := d.instance.PutObject(d.ctx, d.bucket, file, reader, reader.Size(),
		minio.PutObjectOptions{ContentType: mtype.String()})
	return err
}

func (d *minioDriver) PutFile(path string, source contracts.File) (string, error) {
	key, err := cloudFile(path, source)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(source.File())
	if err != nil {
		return "", err
	}
	if err := d.Put(key, string(data)); err != nil {
		return "", err
	}
	return key, nil
}

func (d *minioDriver) PutFileAs(path string, source contracts.File, name string) (string, error) {
	key, err := cloudFileAs(path, source, name)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(source.File())
	if err != nil {
		return "", err
	}
	if err := d.Put(key, string(data)); err != nil {
		return "", err
	}
	return key, nil
}

func (d *minioDriver) Get(file string) (string, error) {
	b, err := d.GetBytes(file)
	return string(b), err
}

func (d *minioDriver) GetBytes(file string) ([]byte, error) {
	obj, err := d.instance.GetObject(d.ctx, d.bucket, file, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close()
	return io.ReadAll(obj)
}

func (d *minioDriver) Exists(file string) bool {
	_, err := d.instance.StatObject(d.ctx, d.bucket, file, minio.StatObjectOptions{})
	return err == nil
}

func (d *minioDriver) Missing(file string) bool { return !d.Exists(file) }

func (d *minioDriver) Url(file string) string {
	realURL := d.url
	if !strings.HasSuffix(realURL, d.bucket) {
		realURL += "/" + d.bucket
	}
	return realURL + "/" + strings.TrimPrefix(file, "/")
}

func (d *minioDriver) TemporaryUrl(file string, t int64) (string, error) {
	file = strings.TrimPrefix(file, "/")
	expireTime := time.Unix(0, t)
	duration := time.Until(expireTime)
	if duration <= 0 {
		return "", fmt.Errorf("[GoFast] minio: expiry time must be in the future")
	}
	u, err := d.instance.PresignedGetObject(d.ctx, d.bucket, file, duration, url.Values{})
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func (d *minioDriver) Copy(oldFile, newFile string) error {
	_, err := d.instance.CopyObject(d.ctx,
		minio.CopyDestOptions{Bucket: d.bucket, Object: newFile},
		minio.CopySrcOptions{Bucket: d.bucket, Object: oldFile},
	)
	return err
}

func (d *minioDriver) Move(oldFile, newFile string) error {
	if err := d.Copy(oldFile, newFile); err != nil {
		return err
	}
	return d.Delete(oldFile)
}

func (d *minioDriver) Delete(files ...string) error {
	ch := make(chan minio.ObjectInfo, len(files))
	for _, f := range files {
		ch <- minio.ObjectInfo{Key: f}
	}
	close(ch)
	for e := range d.instance.RemoveObjects(d.ctx, d.bucket, ch, minio.RemoveObjectsOptions{}) {
		return e.Err
	}
	return nil
}

func (d *minioDriver) Size(file string) (int64, error) {
	info, err := d.instance.StatObject(d.ctx, d.bucket, file, minio.StatObjectOptions{})
	if err != nil {
		return 0, err
	}
	return info.Size, nil
}

func (d *minioDriver) LastModified(file string) (int64, error) {
	info, err := d.instance.StatObject(d.ctx, d.bucket, file, minio.StatObjectOptions{})
	if err != nil {
		return 0, err
	}
	return info.LastModified.Unix(), nil
}

func (d *minioDriver) MimeType(file string) (string, error) {
	info, err := d.instance.StatObject(d.ctx, d.bucket, file, minio.StatObjectOptions{})
	if err != nil {
		return "", err
	}
	return info.ContentType, nil
}

func (d *minioDriver) Path(file string) string { return file }

func (d *minioDriver) MakeDirectory(directory string) error {
	if !strings.HasSuffix(directory, "/") {
		directory += "/"
	}
	return d.Put(directory, "")
}

func (d *minioDriver) DeleteDirectory(directory string) error {
	if !strings.HasSuffix(directory, "/") {
		directory += "/"
	}
	return d.instance.RemoveObject(d.ctx, d.bucket, directory, minio.RemoveObjectOptions{ForceDelete: true})
}

func (d *minioDriver) Files(path string) ([]string, error) {
	var files []string
	vp := validPath(path)
	for obj := range d.instance.ListObjects(d.ctx, d.bucket, minio.ListObjectsOptions{
		Prefix:    vp,
		Recursive: false,
	}) {
		if obj.Err != nil {
			return nil, obj.Err
		}
		if !strings.HasSuffix(obj.Key, "/") {
			files = append(files, strings.TrimPrefix(obj.Key, vp))
		}
	}
	return files, nil
}

func (d *minioDriver) AllFiles(path string) ([]string, error) {
	var files []string
	vp := validPath(path)
	for obj := range d.instance.ListObjects(d.ctx, d.bucket, minio.ListObjectsOptions{
		Prefix:    vp,
		Recursive: true,
	}) {
		if obj.Err != nil {
			return nil, obj.Err
		}
		if !strings.HasSuffix(obj.Key, "/") {
			files = append(files, strings.TrimPrefix(obj.Key, vp))
		}
	}
	return files, nil
}

func (d *minioDriver) Directories(path string) ([]string, error) {
	var dirs []string
	vp := validPath(path)
	for obj := range d.instance.ListObjects(d.ctx, d.bucket, minio.ListObjectsOptions{
		Prefix:    vp,
		Recursive: false,
	}) {
		if obj.Err != nil {
			return nil, obj.Err
		}
		if strings.HasSuffix(obj.Key, "/") {
			dir := strings.TrimPrefix(obj.Key, vp)
			if dir != "" {
				dirs = append(dirs, dir)
			}
		}
	}
	return dirs, nil
}

func (d *minioDriver) AllDirectories(path string) ([]string, error) {
	var dirs []string
	vp := validPath(path)
	for obj := range d.instance.ListObjects(d.ctx, d.bucket, minio.ListObjectsOptions{
		Prefix:    vp,
		Recursive: false,
	}) {
		if obj.Err != nil {
			return nil, obj.Err
		}
		if strings.HasSuffix(obj.Key, "/") {
			key := strings.TrimPrefix(obj.Key, vp)
			if key != "" {
				dirs = append(dirs, key)
				sub, err := d.AllDirectories(obj.Key)
				if err != nil {
					return nil, err
				}
				for _, s := range sub {
					dirs = append(dirs, strings.TrimPrefix(obj.Key+s, vp))
				}
			}
		}
	}
	return dirs, nil
}

func (d *minioDriver) WithContext(ctx context.Context) contracts.StorageDriver {
	clone := *d
	clone.ctx = ctx
	return &clone
}
