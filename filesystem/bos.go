package filesystem

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/baidubce/bce-sdk-go/bce"
	"github.com/baidubce/bce-sdk-go/services/bos"
	"github.com/baidubce/bce-sdk-go/services/bos/api"
	"github.com/zhoudm1743/go-fast-framework/contracts"
)

const bosMaxKeys = 1000

type bosDriver struct {
	ctx      context.Context
	instance *bos.Client
	bucket   string
	diskName string
	url      string
}

func newBosDriver(name string, cfg map[string]any) (*bosDriver, error) {
	key := getString(cfg, "key")
	secret := getString(cfg, "secret")
	bucket := getString(cfg, "bucket")
	endpoint := getString(cfg, "endpoint")
	diskURL := getString(cfg, "url")

	if key == "" || secret == "" || bucket == "" || endpoint == "" || diskURL == "" {
		return nil, fmt.Errorf("[GoFast] bos disk %q: key/secret/bucket/endpoint/url are required", name)
	}

	client, err := bos.NewClient(key, secret, endpoint)
	if err != nil {
		return nil, fmt.Errorf("[GoFast] bos disk %q: init client: %w", name, err)
	}

	return &bosDriver{
		ctx:      context.Background(),
		instance: client,
		bucket:   bucket,
		diskName: name,
		url:      strings.TrimSuffix(diskURL, "/"),
	}, nil
}

func (d *bosDriver) Put(file, content string) error {
	body, err := bce.NewBodyFromString(content)
	if err != nil {
		return err
	}
	_, err = d.instance.BasicPutObject(d.bucket, file, body)
	return err
}

func (d *bosDriver) PutFile(path string, source contracts.File) (string, error) {
	key, err := cloudFile(path, source)
	if err != nil {
		return "", err
	}
	if _, err := d.instance.PutObjectFromFile(d.bucket, key, source.File(), nil); err != nil {
		return "", err
	}
	return key, nil
}

func (d *bosDriver) PutFileAs(path string, source contracts.File, name string) (string, error) {
	key, err := cloudFileAs(path, source, name)
	if err != nil {
		return "", err
	}
	if _, err := d.instance.PutObjectFromFile(d.bucket, key, source.File(), nil); err != nil {
		return "", err
	}
	return key, nil
}

func (d *bosDriver) Get(file string) (string, error) {
	b, err := d.GetBytes(file)
	return string(b), err
}

func (d *bosDriver) GetBytes(file string) ([]byte, error) {
	res, err := d.instance.BasicGetObject(d.bucket, file)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	return io.ReadAll(res.Body)
}

func (d *bosDriver) Exists(file string) bool {
	_, err := d.instance.GetObjectMeta(d.bucket, file)
	return err == nil
}

func (d *bosDriver) Missing(file string) bool { return !d.Exists(file) }

func (d *bosDriver) Url(file string) string {
	return d.url + "/" + strings.TrimPrefix(file, "/")
}

func (d *bosDriver) TemporaryUrl(file string, t int64) (string, error) {
	expireTime := time.Unix(0, t)
	seconds := int64(time.Until(expireTime).Seconds())
	if seconds <= 0 {
		return "", fmt.Errorf("[GoFast] bos: expiry time must be in the future")
	}
	return d.instance.BasicGeneratePresignedUrl(d.bucket, strings.TrimPrefix(file, "/"), int(seconds)), nil
}

func (d *bosDriver) Copy(oldFile, newFile string) error {
	_, err := d.instance.BasicCopyObject(d.bucket, newFile, d.bucket, oldFile)
	return err
}

func (d *bosDriver) Move(oldFile, newFile string) error {
	if err := d.Copy(oldFile, newFile); err != nil {
		return err
	}
	return d.Delete(oldFile)
}

func (d *bosDriver) Delete(files ...string) error {
	for _, f := range files {
		if err := d.instance.DeleteObject(d.bucket, f); err != nil {
			return err
		}
	}
	return nil
}

func (d *bosDriver) Size(file string) (int64, error) {
	meta, err := d.instance.GetObjectMeta(d.bucket, file)
	if err != nil {
		return 0, err
	}
	return meta.ContentLength, nil
}

func (d *bosDriver) LastModified(file string) (int64, error) {
	meta, err := d.instance.GetObjectMeta(d.bucket, file)
	if err != nil {
		return 0, err
	}
	t, err := http.ParseTime(meta.LastModified)
	if err != nil {
		return 0, err
	}
	return t.Unix(), nil
}

func (d *bosDriver) MimeType(file string) (string, error) {
	meta, err := d.instance.GetObjectMeta(d.bucket, file)
	if err != nil {
		return "", err
	}
	return meta.ContentType, nil
}

func (d *bosDriver) Path(file string) string { return file }

func (d *bosDriver) MakeDirectory(directory string) error {
	if !strings.HasSuffix(directory, "/") {
		directory += "/"
	}
	return d.Put(directory, "")
}

func (d *bosDriver) DeleteDirectory(directory string) error {
	if !strings.HasSuffix(directory, "/") {
		directory += "/"
	}
	marker := ""
	for {
		res, err := d.instance.ListObjects(d.bucket, &api.ListObjectsArgs{
			Prefix:  directory,
			Marker:  marker,
			MaxKeys: bosMaxKeys,
		})
		if err != nil {
			return err
		}
		if len(res.Contents) == 0 {
			return nil
		}
		for _, obj := range res.Contents {
			if err := d.instance.DeleteObject(d.bucket, obj.Key); err != nil {
				return err
			}
		}
		if !res.IsTruncated {
			break
		}
		marker = res.NextMarker
	}
	return nil
}

func (d *bosDriver) Files(path string) ([]string, error) {
	var files []string
	vp := validPath(path)
	res, err := d.instance.ListObjects(d.bucket, &api.ListObjectsArgs{
		Prefix:    vp,
		Delimiter: "/",
		MaxKeys:   bosMaxKeys,
	})
	if err != nil {
		return nil, err
	}
	for _, obj := range res.Contents {
		f := strings.TrimPrefix(obj.Key, vp)
		if f != "" {
			files = append(files, f)
		}
	}
	return files, nil
}

func (d *bosDriver) AllFiles(path string) ([]string, error) {
	var files []string
	vp := validPath(path)
	marker := ""
	for {
		res, err := d.instance.ListObjects(d.bucket, &api.ListObjectsArgs{
			Prefix:  vp,
			Marker:  marker,
			MaxKeys: bosMaxKeys,
		})
		if err != nil {
			return nil, err
		}
		for _, obj := range res.Contents {
			if !strings.HasSuffix(obj.Key, "/") {
				files = append(files, strings.TrimPrefix(obj.Key, vp))
			}
		}
		if !res.IsTruncated {
			break
		}
		marker = res.NextMarker
	}
	return files, nil
}

func (d *bosDriver) Directories(path string) ([]string, error) {
	var dirs []string
	vp := validPath(path)
	res, err := d.instance.ListObjects(d.bucket, &api.ListObjectsArgs{
		Prefix:    vp,
		Delimiter: "/",
		MaxKeys:   bosMaxKeys,
	})
	if err != nil {
		return nil, err
	}
	for _, cp := range res.CommonPrefixes {
		dir := strings.TrimPrefix(cp.Prefix, vp)
		if dir != "" {
			dirs = append(dirs, dir)
		}
	}
	return dirs, nil
}

func (d *bosDriver) AllDirectories(path string) ([]string, error) {
	var dirs []string
	vp := validPath(path)
	res, err := d.instance.ListObjects(d.bucket, &api.ListObjectsArgs{
		Prefix:    vp,
		Delimiter: "/",
		MaxKeys:   bosMaxKeys,
	})
	if err != nil {
		return nil, err
	}
	for _, cp := range res.CommonPrefixes {
		prefix := cp.Prefix
		dir := strings.TrimPrefix(prefix, vp)
		if dir != "" {
			dirs = append(dirs, dir)
			sub, err := d.AllDirectories(prefix)
			if err != nil {
				return nil, err
			}
			for _, s := range sub {
				dirs = append(dirs, strings.TrimPrefix(prefix+s, vp))
			}
		}
	}
	return dirs, nil
}

func (d *bosDriver) WithContext(ctx context.Context) contracts.StorageDriver {
	clone := *d
	clone.ctx = ctx
	return &clone
}
