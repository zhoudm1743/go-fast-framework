package filesystem

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/qiniu/go-sdk/v7/auth/qbox"
	qiniuStorage "github.com/qiniu/go-sdk/v7/storage"
	"github.com/zhoudm1743/go-fast-framework/contracts"
)

const qiniuMaxKeys = 1000

type qiniuDriver struct {
	ctx        context.Context
	mac        *qbox.Mac
	bucket     string
	domain     string
	diskName   string
	private    bool
	cfg        *qiniuStorage.Config
	bm         *qiniuStorage.BucketManager
	uploader   *qiniuStorage.FormUploader
	upTokenGen func() string
}

func newQiniuDriver(name string, cfg map[string]any) (*qiniuDriver, error) {
	key := getString(cfg, "key")
	secret := getString(cfg, "secret")
	bucket := getString(cfg, "bucket")
	domain := getString(cfg, "url")
	regionID := getString(cfg, "region")
	private := getBool(cfg, "private")

	if key == "" || secret == "" || bucket == "" || domain == "" {
		return nil, fmt.Errorf("[GoFast] qiniu disk %q: key/secret/bucket/url are required", name)
	}

	mac := qbox.NewMac(key, secret)

	storCfg := qiniuStorage.NewConfig()
	if regionID != "" {
		if region, ok := qiniuStorage.GetRegionByID(qiniuStorage.RegionID(regionID)); ok {
			storCfg.Region = &region
		}
	}

	bm := qiniuStorage.NewBucketManager(mac, storCfg)
	uploader := qiniuStorage.NewFormUploader(storCfg)

	return &qiniuDriver{
		ctx:      context.Background(),
		mac:      mac,
		bucket:   bucket,
		domain:   strings.TrimSuffix(domain, "/"),
		diskName: name,
		private:  private,
		cfg:      storCfg,
		bm:       bm,
		uploader: uploader,
		upTokenGen: func() string {
			putPolicy := qiniuStorage.PutPolicy{Scope: bucket}
			return putPolicy.UploadToken(mac)
		},
	}, nil
}

func (d *qiniuDriver) Put(file, content string) error {
	tmp, err := os.CreateTemp("", "go-fast-qiniu-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	return d.uploadFile(file, tmp.Name())
}

func (d *qiniuDriver) PutFile(path string, source contracts.File) (string, error) {
	key, err := cloudFile(path, source)
	if err != nil {
		return "", err
	}
	if err := d.uploadFile(key, source.File()); err != nil {
		return "", err
	}
	return key, nil
}

func (d *qiniuDriver) PutFileAs(path string, source contracts.File, name string) (string, error) {
	key, err := cloudFileAs(path, source, name)
	if err != nil {
		return "", err
	}
	if err := d.uploadFile(key, source.File()); err != nil {
		return "", err
	}
	return key, nil
}

func (d *qiniuDriver) uploadFile(key, localFile string) error {
	ret := qiniuStorage.PutRet{}
	return d.uploader.PutFile(d.ctx, &ret, d.upTokenGen(), key, localFile, nil)
}

func (d *qiniuDriver) Get(file string) (string, error) {
	b, err := d.GetBytes(file)
	return string(b), err
}

func (d *qiniuDriver) GetBytes(file string) ([]byte, error) {
	url := d.Url(file)
	req, err := http.NewRequestWithContext(d.ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (d *qiniuDriver) Exists(file string) bool {
	_, err := d.bm.Stat(d.bucket, file)
	return err == nil
}

func (d *qiniuDriver) Missing(file string) bool { return !d.Exists(file) }

func (d *qiniuDriver) Url(file string) string {
	key := strings.TrimPrefix(file, "/")
	if d.private {
		deadline := time.Now().Add(time.Hour).Unix()
		return qiniuStorage.MakePrivateURL(d.mac, d.domain, key, deadline)
	}
	return d.domain + "/" + key
}

func (d *qiniuDriver) TemporaryUrl(file string, t int64) (string, error) {
	expireTime := time.Unix(0, t)
	seconds := int64(time.Until(expireTime).Seconds())
	if seconds <= 0 {
		return "", fmt.Errorf("[GoFast] qiniu: expiry time must be in the future")
	}
	deadline := time.Now().Unix() + seconds
	return qiniuStorage.MakePrivateURL(d.mac, d.domain, strings.TrimPrefix(file, "/"), deadline), nil
}

func (d *qiniuDriver) Copy(oldFile, newFile string) error {
	return d.bm.Copy(d.bucket, oldFile, d.bucket, newFile, true)
}

func (d *qiniuDriver) Move(oldFile, newFile string) error {
	return d.bm.Move(d.bucket, oldFile, d.bucket, newFile, true)
}

func (d *qiniuDriver) Delete(files ...string) error {
	for _, f := range files {
		if err := d.bm.Delete(d.bucket, f); err != nil {
			return err
		}
	}
	return nil
}

func (d *qiniuDriver) Size(file string) (int64, error) {
	info, err := d.bm.Stat(d.bucket, file)
	if err != nil {
		return 0, err
	}
	return info.Fsize, nil
}

func (d *qiniuDriver) LastModified(file string) (int64, error) {
	info, err := d.bm.Stat(d.bucket, file)
	if err != nil {
		return 0, err
	}
	return info.PutTime >> 32, nil
}

func (d *qiniuDriver) MimeType(file string) (string, error) {
	info, err := d.bm.Stat(d.bucket, file)
	if err != nil {
		return "", err
	}
	return info.MimeType, nil
}

func (d *qiniuDriver) Path(file string) string { return file }

func (d *qiniuDriver) MakeDirectory(directory string) error {
	if !strings.HasSuffix(directory, "/") {
		directory += "/"
	}
	return d.Put(directory, "")
}

func (d *qiniuDriver) DeleteDirectory(directory string) error {
	if !strings.HasSuffix(directory, "/") {
		directory += "/"
	}
	files, err := d.AllFiles(directory)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}
	// 拼接回完整 key
	keys := make([]string, 0, len(files))
	for _, f := range files {
		keys = append(keys, directory+f)
	}
	return d.Delete(keys...)
}

func (d *qiniuDriver) Files(path string) ([]string, error) {
	return d.listFiles(path, "/")
}

func (d *qiniuDriver) AllFiles(path string) ([]string, error) {
	return d.listFiles(path, "")
}

func (d *qiniuDriver) Directories(path string) ([]string, error) {
	return d.listDirectories(path, false)
}

func (d *qiniuDriver) AllDirectories(path string) ([]string, error) {
	return d.listDirectories(path, true)
}

func (d *qiniuDriver) listFiles(path, delimiter string) ([]string, error) {
	prefix := validPath(path)
	var result []string
	marker := ""
	for {
		items, _, nextMarker, hasNext, err := d.bm.ListFiles(d.bucket, prefix, delimiter, marker, qiniuMaxKeys)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			key := strings.TrimPrefix(item.Key, prefix)
			if key != "" {
				result = append(result, key)
			}
		}
		if !hasNext {
			break
		}
		marker = nextMarker
	}
	return result, nil
}

func (d *qiniuDriver) listDirectories(path string, recursive bool) ([]string, error) {
	prefix := validPath(path)
	var result []string
	marker := ""
	for {
		_, commonPrefixes, nextMarker, hasNext, err := d.bm.ListFiles(d.bucket, prefix, "/", marker, qiniuMaxKeys)
		if err != nil {
			return nil, err
		}
		for _, cp := range commonPrefixes {
			dir := strings.TrimPrefix(cp, prefix)
			if dir != "" {
				result = append(result, dir)
				if recursive {
					sub, err := d.listDirectories(cp, true)
					if err != nil {
						return nil, err
					}
					for _, s := range sub {
						result = append(result, strings.TrimPrefix(cp+s, prefix))
					}
				}
			}
		}
		if !hasNext {
			break
		}
		marker = nextMarker
	}
	return result, nil
}

func (d *qiniuDriver) WithContext(ctx context.Context) contracts.StorageDriver {
	clone := *d
	clone.ctx = ctx
	return &clone
}
