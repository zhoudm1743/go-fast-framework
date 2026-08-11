package filesystem

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/gabriel-vasile/mimetype"
	"github.com/zhoudm1743/go-fast-framework/contracts"
)

type s3Driver struct {
	ctx             context.Context
	instance        *s3.Client
	bucket          string
	diskName        string
	url             string
	cdn             string
	objectCannedACL string
}

func newS3Driver(name string, cfg map[string]any) (*s3Driver, error) {
	key := getString(cfg, "key")
	secret := getString(cfg, "secret")
	region := getString(cfg, "region")
	bucket := getString(cfg, "bucket")
	diskURL := getString(cfg, "url")
	token := getString(cfg, "token")
	endpoint := getString(cfg, "endpoint")
	cdn := getString(cfg, "cdn")
	objectCannedACL := getString(cfg, "object_canned_acl")
	usePathStyle := getBool(cfg, "use_path_style")

	if key == "" || secret == "" || region == "" || bucket == "" || diskURL == "" {
		return nil, fmt.Errorf("[GoFast] s3 disk %q: key/secret/region/bucket/url are required", name)
	}

	opts := s3.Options{
		Region: region,
		Credentials: aws.NewCredentialsCache(
			credentials.NewStaticCredentialsProvider(key, secret, token),
		),
		UsePathStyle: usePathStyle,
	}
	if endpoint != "" {
		opts.BaseEndpoint = aws.String(endpoint)
	}

	client := s3.New(opts)

	return &s3Driver{
		ctx:             context.Background(),
		instance:        client,
		bucket:          bucket,
		diskName:        name,
		url:             strings.TrimSuffix(diskURL, "/"),
		cdn:             strings.TrimSuffix(cdn, "/"),
		objectCannedACL: objectCannedACL,
	}, nil
}

func (d *s3Driver) Put(file, content string) error {
	mtype := mimetype.Detect([]byte(content))
	input := &s3.PutObjectInput{
		Bucket:        aws.String(d.bucket),
		Key:           aws.String(file),
		Body:          strings.NewReader(content),
		ContentLength: aws.Int64(int64(len(content))),
		ContentType:   aws.String(mtype.String()),
	}
	if d.objectCannedACL != "" {
		input.ACL = s3types.ObjectCannedACL(d.objectCannedACL)
	}
	_, err := d.instance.PutObject(d.ctx, input)
	return err
}

func (d *s3Driver) PutFile(path string, source contracts.File) (string, error) {
	key, err := cloudFile(path, source)
	if err != nil {
		return "", err
	}
	if err := d.uploadFromPath(key, source.File()); err != nil {
		return "", err
	}
	return key, nil
}

func (d *s3Driver) PutFileAs(path string, source contracts.File, name string) (string, error) {
	key, err := cloudFileAs(path, source, name)
	if err != nil {
		return "", err
	}
	if err := d.uploadFromPath(key, source.File()); err != nil {
		return "", err
	}
	return key, nil
}

func (d *s3Driver) uploadFromPath(key, filePath string) error {
	// 使用 mimetype 检测文件类型并上传
	mtype, err := mimetype.DetectFile(filePath)
	if err != nil {
		return err
	}
	f, err := openFileForUpload(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	size, err := fileSize(filePath)
	if err != nil {
		return err
	}
	input := &s3.PutObjectInput{
		Bucket:        aws.String(d.bucket),
		Key:           aws.String(key),
		Body:          f,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(mtype.String()),
	}
	if d.objectCannedACL != "" {
		input.ACL = s3types.ObjectCannedACL(d.objectCannedACL)
	}
	_, err = d.instance.PutObject(d.ctx, input)
	return err
}

func (d *s3Driver) Get(file string) (string, error) {
	b, err := d.GetBytes(file)
	return string(b), err
}

func (d *s3Driver) GetBytes(file string) ([]byte, error) {
	resp, err := d.instance.GetObject(d.ctx, &s3.GetObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(file),
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (d *s3Driver) Exists(file string) bool {
	_, err := d.instance.HeadObject(d.ctx, &s3.HeadObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(file),
	})
	return err == nil
}

func (d *s3Driver) Missing(file string) bool { return !d.Exists(file) }

func (d *s3Driver) Url(file string) string {
	base := d.url
	if d.cdn != "" {
		base = d.cdn
	}
	return base + "/" + strings.TrimPrefix(file, "/")
}

func (d *s3Driver) TemporaryUrl(file string, t int64) (string, error) {
	expireTime := time.Unix(0, t)
	duration := time.Until(expireTime)
	if duration <= 0 {
		return "", fmt.Errorf("[GoFast] s3: expiry time must be in the future")
	}
	presign := s3.NewPresignClient(d.instance)
	result, err := presign.PresignGetObject(d.ctx, &s3.GetObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(file),
	}, func(o *s3.PresignOptions) {
		o.Expires = duration
	})
	if err != nil {
		return "", err
	}
	return result.URL, nil
}

func (d *s3Driver) Copy(oldFile, newFile string) error {
	_, err := d.instance.CopyObject(d.ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(d.bucket),
		CopySource: aws.String(d.bucket + "/" + oldFile),
		Key:        aws.String(newFile),
	})
	return err
}

func (d *s3Driver) Move(oldFile, newFile string) error {
	if err := d.Copy(oldFile, newFile); err != nil {
		return err
	}
	return d.Delete(oldFile)
}

func (d *s3Driver) Delete(files ...string) error {
	objs := make([]s3types.ObjectIdentifier, 0, len(files))
	for _, f := range files {
		objs = append(objs, s3types.ObjectIdentifier{Key: aws.String(f)})
	}
	_, err := d.instance.DeleteObjects(d.ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(d.bucket),
		Delete: &s3types.Delete{Objects: objs, Quiet: aws.Bool(true)},
	})
	return err
}

func (d *s3Driver) Size(file string) (int64, error) {
	resp, err := d.instance.HeadObject(d.ctx, &s3.HeadObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(file),
	})
	if err != nil {
		return 0, err
	}
	return aws.ToInt64(resp.ContentLength), nil
}

func (d *s3Driver) LastModified(file string) (int64, error) {
	resp, err := d.instance.HeadObject(d.ctx, &s3.HeadObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(file),
	})
	if err != nil {
		return 0, err
	}
	return aws.ToTime(resp.LastModified).Unix(), nil
}

func (d *s3Driver) MimeType(file string) (string, error) {
	resp, err := d.instance.HeadObject(d.ctx, &s3.HeadObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(file),
	})
	if err != nil {
		return "", err
	}
	return aws.ToString(resp.ContentType), nil
}

func (d *s3Driver) Path(file string) string { return file }

func (d *s3Driver) MakeDirectory(directory string) error {
	if !strings.HasSuffix(directory, "/") {
		directory += "/"
	}
	return d.Put(directory, "")
}

func (d *s3Driver) DeleteDirectory(directory string) error {
	if !strings.HasSuffix(directory, "/") {
		directory += "/"
	}
	var token *string
	for {
		resp, err := d.instance.ListObjectsV2(d.ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(d.bucket),
			Prefix:            aws.String(directory),
			ContinuationToken: token,
		})
		if err != nil {
			return err
		}
		if len(resp.Contents) == 0 {
			break
		}
		for _, item := range resp.Contents {
			if _, err := d.instance.DeleteObject(d.ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(d.bucket),
				Key:    item.Key,
			}); err != nil {
				return err
			}
		}
		if !aws.ToBool(resp.IsTruncated) {
			break
		}
		token = resp.NextContinuationToken
	}
	return nil
}

func (d *s3Driver) Files(path string) ([]string, error) {
	var files []string
	vp := validPath(path)
	resp, err := d.instance.ListObjectsV2(d.ctx, &s3.ListObjectsV2Input{
		Bucket:    aws.String(d.bucket),
		Prefix:    aws.String(vp),
		Delimiter: aws.String("/"),
	})
	if err != nil {
		return nil, err
	}
	for _, obj := range resp.Contents {
		f := strings.TrimPrefix(aws.ToString(obj.Key), vp)
		if f != "" {
			files = append(files, f)
		}
	}
	return files, nil
}

func (d *s3Driver) AllFiles(path string) ([]string, error) {
	var files []string
	vp := validPath(path)
	resp, err := d.instance.ListObjectsV2(d.ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(d.bucket),
		Prefix: aws.String(vp),
	})
	if err != nil {
		return nil, err
	}
	for _, obj := range resp.Contents {
		key := aws.ToString(obj.Key)
		if !strings.HasSuffix(key, "/") {
			files = append(files, strings.TrimPrefix(key, vp))
		}
	}
	return files, nil
}

func (d *s3Driver) Directories(path string) ([]string, error) {
	var dirs []string
	vp := validPath(path)
	resp, err := d.instance.ListObjectsV2(d.ctx, &s3.ListObjectsV2Input{
		Bucket:    aws.String(d.bucket),
		Prefix:    aws.String(vp),
		Delimiter: aws.String("/"),
	})
	if err != nil {
		return nil, err
	}
	for _, cp := range resp.CommonPrefixes {
		dirs = append(dirs, strings.TrimPrefix(aws.ToString(cp.Prefix), vp))
	}
	return dirs, nil
}

func (d *s3Driver) AllDirectories(path string) ([]string, error) {
	var dirs []string
	vp := validPath(path)
	resp, err := d.instance.ListObjectsV2(d.ctx, &s3.ListObjectsV2Input{
		Bucket:    aws.String(d.bucket),
		Prefix:    aws.String(vp),
		Delimiter: aws.String("/"),
	})
	if err != nil {
		return nil, err
	}
	for _, cp := range resp.CommonPrefixes {
		prefix := aws.ToString(cp.Prefix)
		dirs = append(dirs, strings.TrimPrefix(prefix, vp))
		sub, err := d.AllDirectories(prefix)
		if err != nil {
			return nil, err
		}
		for _, s := range sub {
			dirs = append(dirs, strings.TrimPrefix(prefix+s, vp))
		}
	}
	return dirs, nil
}

func (d *s3Driver) WithContext(ctx context.Context) contracts.StorageDriver {
	clone := *d
	clone.ctx = ctx
	return &clone
}
