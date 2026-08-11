package filesystem

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/zhoudm1743/go-fast-framework/contracts"
)

type ossDriver struct {
	ctx      context.Context
	client   *oss.Client
	bucket   *oss.Bucket
	diskName string
	url      string
	endpoint string
	key      string
	secret   string
	bktName  string
}

func newOssDriver(name string, cfg map[string]any) (*ossDriver, error) {
	key := getString(cfg, "key")
	secret := getString(cfg, "secret")
	bktName := getString(cfg, "bucket")
	url := getString(cfg, "url")
	endpoint := getString(cfg, "endpoint")

	if key == "" || secret == "" || bktName == "" || url == "" || endpoint == "" {
		return nil, fmt.Errorf("[GoFast] oss disk %q: key/secret/bucket/url/endpoint are required", name)
	}

	client, err := oss.New(endpoint, key, secret)
	if err != nil {
		return nil, fmt.Errorf("[GoFast] oss disk %q: init client: %w", name, err)
	}

	bucket, err := client.Bucket(bktName)
	if err != nil {
		return nil, fmt.Errorf("[GoFast] oss disk %q: init bucket: %w", name, err)
	}

	return &ossDriver{
		ctx:      context.Background(),
		client:   client,
		bucket:   bucket,
		diskName: name,
		url:      strings.TrimSuffix(url, "/"),
		endpoint: endpoint,
		key:      key,
		secret:   secret,
		bktName:  bktName,
	}, nil
}

func (d *ossDriver) Put(file, content string) error {
	tmp, err := os.CreateTemp("", "go-fast-oss-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	return d.bucket.PutObjectFromFile(file, tmp.Name())
}

func (d *ossDriver) PutFile(path string, source contracts.File) (string, error) {
	key, err := cloudFile(path, source)
	if err != nil {
		return "", err
	}
	if err := d.bucket.PutObjectFromFile(key, source.File()); err != nil {
		return "", err
	}
	return key, nil
}

func (d *ossDriver) PutFileAs(path string, source contracts.File, name string) (string, error) {
	key, err := cloudFileAs(path, source, name)
	if err != nil {
		return "", err
	}
	if err := d.bucket.PutObjectFromFile(key, source.File()); err != nil {
		return "", err
	}
	return key, nil
}

func (d *ossDriver) Get(file string) (string, error) {
	b, err := d.GetBytes(file)
	return string(b), err
}

func (d *ossDriver) GetBytes(file string) ([]byte, error) {
	res, err := d.bucket.GetObject(file)
	if err != nil {
		return nil, err
	}
	defer res.Close()
	return io.ReadAll(res)
}

func (d *ossDriver) Exists(file string) bool {
	ok, err := d.bucket.IsObjectExist(file)
	return err == nil && ok
}

func (d *ossDriver) Missing(file string) bool { return !d.Exists(file) }

func (d *ossDriver) Url(file string) string {
	return d.url + "/" + strings.TrimPrefix(file, "/")
}

func (d *ossDriver) TemporaryUrl(file string, t int64) (string, error) {
	expireTime := time.Unix(0, t)
	seconds := int64(time.Until(expireTime).Seconds())
	if seconds <= 0 {
		return "", fmt.Errorf("[GoFast] oss: expiry time must be in the future")
	}
	return d.bucket.SignURL(file, oss.HTTPGet, seconds)
}

func (d *ossDriver) Copy(oldFile, newFile string) error {
	_, err := d.bucket.CopyObject(oldFile, newFile)
	return err
}

func (d *ossDriver) Move(oldFile, newFile string) error {
	if err := d.Copy(oldFile, newFile); err != nil {
		return err
	}
	return d.Delete(oldFile)
}

func (d *ossDriver) Delete(files ...string) error {
	_, err := d.bucket.DeleteObjects(files)
	return err
}

func (d *ossDriver) Size(file string) (int64, error) {
	props, err := d.bucket.GetObjectDetailedMeta(file)
	if err != nil {
		return 0, err
	}
	lens := props["Content-Length"]
	if len(lens) == 0 {
		return 0, nil
	}
	return strconv.ParseInt(lens[0], 10, 64)
}

func (d *ossDriver) LastModified(file string) (int64, error) {
	headers, err := d.bucket.GetObjectDetailedMeta(file)
	if err != nil {
		return 0, err
	}
	t, err := time.Parse(time.RFC1123, headers.Get("Last-Modified"))
	if err != nil {
		return 0, err
	}
	return t.Unix(), nil
}

func (d *ossDriver) MimeType(file string) (string, error) {
	headers, err := d.bucket.GetObjectDetailedMeta(file)
	if err != nil {
		return "", err
	}
	return headers.Get("Content-Type"), nil
}

func (d *ossDriver) Path(file string) string { return file }

func (d *ossDriver) MakeDirectory(directory string) error {
	if !strings.HasSuffix(directory, "/") {
		directory += "/"
	}
	return d.bucket.PutObject(directory, bytes.NewReader(nil))
}

func (d *ossDriver) DeleteDirectory(directory string) error {
	if !strings.HasSuffix(directory, "/") {
		directory += "/"
	}
	marker := oss.Marker("")
	prefix := oss.Prefix(directory)
	for {
		lor, err := d.bucket.ListObjects(marker, prefix)
		if err != nil {
			return err
		}
		if len(lor.Objects) == 0 {
			return nil
		}
		keys := make([]string, 0, len(lor.Objects))
		for _, obj := range lor.Objects {
			keys = append(keys, obj.Key)
		}
		if _, err := d.bucket.DeleteObjects(keys, oss.DeleteObjectsQuiet(true)); err != nil {
			return err
		}
		if !lor.IsTruncated {
			break
		}
		prefix = oss.Prefix(lor.Prefix)
		marker = oss.Marker(lor.NextMarker)
	}
	return nil
}

func (d *ossDriver) Files(path string) ([]string, error) {
	var files []string
	vp := validPath(path)
	res, err := d.bucket.ListObjectsV2(oss.MaxKeys(cloudMaxKeys), oss.Prefix(vp), oss.Delimiter("/"))
	if err != nil {
		return nil, err
	}
	for _, obj := range res.Objects {
		f := strings.TrimPrefix(obj.Key, vp)
		if f != "" {
			files = append(files, f)
		}
	}
	return files, nil
}

func (d *ossDriver) AllFiles(path string) ([]string, error) {
	var files []string
	vp := validPath(path)
	res, err := d.bucket.ListObjectsV2(oss.MaxKeys(cloudMaxKeys), oss.Prefix(vp))
	if err != nil {
		return nil, err
	}
	for _, obj := range res.Objects {
		if !strings.HasSuffix(obj.Key, "/") {
			files = append(files, strings.TrimPrefix(obj.Key, vp))
		}
	}
	return files, nil
}

func (d *ossDriver) Directories(path string) ([]string, error) {
	var dirs []string
	vp := validPath(path)
	res, err := d.bucket.ListObjectsV2(oss.MaxKeys(cloudMaxKeys), oss.Prefix(vp), oss.Delimiter("/"))
	if err != nil {
		return nil, err
	}
	for _, cp := range res.CommonPrefixes {
		dir := strings.TrimPrefix(cp, vp)
		if dir != "" {
			dirs = append(dirs, dir)
		}
	}
	return dirs, nil
}

func (d *ossDriver) AllDirectories(path string) ([]string, error) {
	var dirs []string
	vp := validPath(path)
	res, err := d.bucket.ListObjectsV2(oss.MaxKeys(cloudMaxKeys), oss.Prefix(vp), oss.Delimiter("/"))
	if err != nil {
		return nil, err
	}
	for _, cp := range res.CommonPrefixes {
		dir := strings.TrimPrefix(cp, vp)
		dirs = append(dirs, dir)
		sub, err := d.AllDirectories(cp)
		if err != nil {
			return nil, err
		}
		for _, s := range sub {
			dirs = append(dirs, strings.TrimPrefix(cp+s, vp))
		}
	}
	return dirs, nil
}

func (d *ossDriver) WithContext(ctx context.Context) contracts.StorageDriver {
	clone := *d
	clone.ctx = ctx
	return &clone
}
