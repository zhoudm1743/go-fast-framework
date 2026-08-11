package filesystem

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tencentyun/cos-go-sdk-v5"
	"github.com/zhoudm1743/go-fast-framework/contracts"
)

type cosDriver struct {
	ctx             context.Context
	instance        *cos.Client
	diskName        string
	baseURL         string
	accessKeyID     string
	accessKeySecret string
}

func newCosDriver(name string, cfg map[string]any) (*cosDriver, error) {
	key := getString(cfg, "key")
	secret := getString(cfg, "secret")
	cosURL := getString(cfg, "url")

	if key == "" || secret == "" || cosURL == "" {
		return nil, fmt.Errorf("[GoFast] cos disk %q: key/secret/url are required", name)
	}

	u, err := url.Parse(cosURL)
	if err != nil {
		return nil, fmt.Errorf("[GoFast] cos disk %q: parse url: %w", name, err)
	}

	client := cos.NewClient(&cos.BaseURL{BucketURL: u}, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  key,
			SecretKey: secret,
		},
	})

	return &cosDriver{
		ctx:             context.Background(),
		instance:        client,
		diskName:        name,
		baseURL:         strings.TrimSuffix(cosURL, "/"),
		accessKeyID:     key,
		accessKeySecret: secret,
	}, nil
}

func (d *cosDriver) Put(file, content string) error {
	tmp, err := os.CreateTemp("", "go-fast-cos-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	_, _, err = d.instance.Object.Upload(d.ctx, file, tmp.Name(), nil)
	return err
}

func (d *cosDriver) PutFile(path string, source contracts.File) (string, error) {
	key, err := cloudFile(path, source)
	if err != nil {
		return "", err
	}
	if _, _, err := d.instance.Object.Upload(d.ctx, key, source.File(), nil); err != nil {
		return "", err
	}
	return key, nil
}

func (d *cosDriver) PutFileAs(path string, source contracts.File, name string) (string, error) {
	key, err := cloudFileAs(path, source, name)
	if err != nil {
		return "", err
	}
	if _, _, err := d.instance.Object.Upload(d.ctx, key, source.File(), nil); err != nil {
		return "", err
	}
	return key, nil
}

func (d *cosDriver) Get(file string) (string, error) {
	b, err := d.GetBytes(file)
	return string(b), err
}

func (d *cosDriver) GetBytes(file string) ([]byte, error) {
	resp, err := d.instance.Object.Get(d.ctx, file, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (d *cosDriver) Exists(file string) bool {
	ok, err := d.instance.Object.IsExist(d.ctx, file)
	return err == nil && ok
}

func (d *cosDriver) Missing(file string) bool { return !d.Exists(file) }

func (d *cosDriver) Url(file string) string {
	return d.instance.Object.GetObjectURL(file).String()
}

func (d *cosDriver) TemporaryUrl(file string, t int64) (string, error) {
	expireTime := time.Unix(0, t)
	duration := time.Until(expireTime)
	if duration <= 0 {
		return "", fmt.Errorf("[GoFast] cos: expiry time must be in the future")
	}
	u, err := d.instance.Object.GetPresignedURL(d.ctx, "GET", file, d.accessKeyID, d.accessKeySecret, duration, nil)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func (d *cosDriver) Copy(oldFile, newFile string) error {
	srcURL := strings.ReplaceAll(
		strings.ReplaceAll(
			strings.TrimSuffix(d.baseURL, "/")+"/"+strings.TrimPrefix(oldFile, "/"),
			"https://", "",
		),
		"http://", "",
	)
	_, _, err := d.instance.Object.Copy(d.ctx, newFile, srcURL, nil)
	return err
}

func (d *cosDriver) Move(oldFile, newFile string) error {
	if err := d.Copy(oldFile, newFile); err != nil {
		return err
	}
	return d.Delete(oldFile)
}

func (d *cosDriver) Delete(files ...string) error {
	obs := make([]cos.Object, 0, len(files))
	for _, f := range files {
		obs = append(obs, cos.Object{Key: f})
	}
	_, _, err := d.instance.Object.DeleteMulti(d.ctx, &cos.ObjectDeleteMultiOptions{
		Objects: obs,
		Quiet:   true,
	})
	return err
}

func (d *cosDriver) Size(file string) (int64, error) {
	resp, err := d.instance.Object.Head(d.ctx, file, nil)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
}

func (d *cosDriver) LastModified(file string) (int64, error) {
	resp, err := d.instance.Object.Head(d.ctx, file, nil)
	if err != nil {
		return 0, err
	}
	t, err := http.ParseTime(resp.Header.Get("Last-Modified"))
	if err != nil {
		return 0, err
	}
	return t.Unix(), nil
}

func (d *cosDriver) MimeType(file string) (string, error) {
	resp, err := d.instance.Object.Head(d.ctx, file, nil)
	if err != nil {
		return "", err
	}
	return resp.Header.Get("Content-Type"), nil
}

func (d *cosDriver) Path(file string) string { return file }

func (d *cosDriver) MakeDirectory(directory string) error {
	if !strings.HasSuffix(directory, "/") {
		directory += "/"
	}
	_, err := d.instance.Object.Put(d.ctx, directory, strings.NewReader(""), nil)
	return err
}

func (d *cosDriver) DeleteDirectory(directory string) error {
	if !strings.HasSuffix(directory, "/") {
		directory += "/"
	}
	var marker string
	opt := &cos.BucketGetOptions{Prefix: directory, MaxKeys: cloudMaxKeys}
	for {
		opt.Marker = marker
		res, _, err := d.instance.Bucket.Get(d.ctx, opt)
		if err != nil {
			return err
		}
		if len(res.Contents) == 0 {
			return nil
		}
		for _, c := range res.Contents {
			if _, err := d.instance.Object.Delete(d.ctx, c.Key); err != nil {
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

func (d *cosDriver) Files(path string) ([]string, error) {
	var files []string
	vp := validPath(path)
	var marker string
	opt := &cos.BucketGetOptions{Prefix: vp, Delimiter: "/", MaxKeys: cloudMaxKeys}
	for {
		opt.Marker = marker
		v, _, err := d.instance.Bucket.Get(d.ctx, opt)
		if err != nil {
			return nil, err
		}
		for _, c := range v.Contents {
			f := strings.TrimPrefix(c.Key, vp)
			if f != "" {
				files = append(files, f)
			}
		}
		if !v.IsTruncated {
			break
		}
		marker = v.NextMarker
	}
	return files, nil
}

func (d *cosDriver) AllFiles(path string) ([]string, error) {
	var files []string
	vp := validPath(path)
	var marker string
	opt := &cos.BucketGetOptions{Prefix: vp, MaxKeys: cloudMaxKeys}
	for {
		opt.Marker = marker
		v, _, err := d.instance.Bucket.Get(d.ctx, opt)
		if err != nil {
			return nil, err
		}
		for _, c := range v.Contents {
			if !strings.HasSuffix(c.Key, "/") {
				files = append(files, strings.TrimPrefix(c.Key, vp))
			}
		}
		if !v.IsTruncated {
			break
		}
		marker = v.NextMarker
	}
	return files, nil
}

func (d *cosDriver) Directories(path string) ([]string, error) {
	var dirs []string
	vp := validPath(path)
	var marker string
	opt := &cos.BucketGetOptions{Prefix: vp, Delimiter: "/", MaxKeys: cloudMaxKeys}
	for {
		opt.Marker = marker
		v, _, err := d.instance.Bucket.Get(d.ctx, opt)
		if err != nil {
			return nil, err
		}
		for _, cp := range v.CommonPrefixes {
			dir := strings.TrimPrefix(cp, vp)
			if dir != "" {
				dirs = append(dirs, dir)
			}
		}
		if !v.IsTruncated {
			break
		}
		marker = v.NextMarker
	}
	return dirs, nil
}

func (d *cosDriver) AllDirectories(path string) ([]string, error) {
	var dirs []string
	vp := validPath(path)
	var marker string
	opt := &cos.BucketGetOptions{Prefix: vp, Delimiter: "/", MaxKeys: cloudMaxKeys}
	for {
		opt.Marker = marker
		v, _, err := d.instance.Bucket.Get(d.ctx, opt)
		if err != nil {
			return nil, err
		}
		for _, cp := range v.CommonPrefixes {
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
		if !v.IsTruncated {
			break
		}
		marker = v.NextMarker
	}
	return dirs, nil
}

func (d *cosDriver) WithContext(ctx context.Context) contracts.StorageDriver {
	clone := *d
	clone.ctx = ctx
	return &clone
}
