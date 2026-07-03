package minio

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/util"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/zap"
)

var (
	initialized bool
	client      *minio.Client
	mu          sync.RWMutex
)

// PutOptions represents upload options.
type PutOptions struct {
	ContentType     string            // MIME type, auto-detected if empty
	Metadata        map[string]string // custom metadata
	ContentEncoding string
	CacheControl    string
	Bucket          string // overwrite the bucket name set in config.
	Size            int64
}

// GetOptions represents download options.
type GetOptions struct {
	VersionID string // object version ID when versioning is enabled
	Range     string // HTTP range header, e.g. "bytes=0-1024"
	Bucket    string // overwrite the bucket name set in config.
}

// RemoveOptions configures delete behavior.
type RemoveOptions struct {
	ForceDelete bool
	VersionID   string
	Bucket      string
}

// ObjectInfo describes object metadata.
type ObjectInfo struct {
	Key          string
	Size         int64
	ContentType  string
	ETag         string
	LastModified time.Time
	Metadata     map[string]string
}

// ExistsOptions configures existence checks.
type ExistsOptions struct {
	VersionID string
	Bucket    string
}

// StatOptions configures stat lookups.
type StatOptions struct {
	VersionID string
	Bucket    string
}

// ListOptions configures object listing.
type ListOptions struct {
	Prefix    string
	Recursive bool
	Bucket    string
}

// CopyOptions configures object copy operations.
type CopyOptions struct {
	Bucket string
}

// Init initializes the global MinIO client.
// It reads MinIO configuration from config.App.Minio.
// If MinIO is not enabled, it returns nil.
// The function is thread-safe and ensures the client is initialized only once.
func Init() (err error) {
	cfg := config.App.Minio
	if !cfg.Enabled {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	if initialized {
		return nil
	}

	newClient, newErr := New(cfg)
	if newErr != nil {
		return errors.Wrap(newErr, "failed to create minio client")
	}

	// Try to establish a connection to MinIO and verify the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Multiple buckets separated by comma.
	buckets := strings.FieldsFunc(cfg.Bucket, func(r rune) bool { return r == ',' })
	for _, bucket := range buckets {
		bucket = strings.TrimSpace(bucket)
		if len(bucket) == 0 {
			continue
		}
		if err := ensureBucket(ctx, newClient, bucket); err != nil {
			return err
		}
	}

	zap.S().Infow("successfully connected to minio", "endpoint", cfg.Endpoint, "bucket", cfg.Bucket, "region", cfg.Region)

	client = newClient
	initialized = true
	return nil
}

// New returns a new MinIO client with given configuration.
func New(cfg config.Minio) (cli *minio.Client, err error) {
	if cfg.Endpoint == "" {
		return nil, errors.New("minio endpoint is empty")
	}

	// Set up credentials options
	var creds *credentials.Credentials
	switch {
	case cfg.UseIAM:
		// Use IAM based credentials
		creds = credentials.NewIAM(cfg.IAMEndpoint)
	case cfg.UseSTS:
		// Use STS based credentials
		if creds, err = credentials.NewSTSAssumeRole(cfg.STSEndpoint, credentials.STSAssumeRoleOptions{
			AccessKey: cfg.AccessKey,
			SecretKey: cfg.SecretKey,
		}); err != nil {
			return nil, errors.Wrap(err, "failed to create sts assume role credentials")
		}
	case cfg.SessionToken != "":
		// Use temporary credentials with session token
		creds = credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, cfg.SessionToken)
	default:
		// Use standard access/secret key
		creds = credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, "")
	}

	// Create MinIO client opts
	opts := &minio.Options{
		Creds:  creds,
		Secure: cfg.Secure,
		Region: cfg.Region,
	}
	// Configure transport with TLS if enabled
	if cfg.TLSEnabled {
		var tlsConfig *tls.Config
		var transport *http.Transport
		if tlsConfig, err = util.BuildTLSConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile, cfg.InsecureSkipVerify); err != nil {
			return nil, errors.Wrap(err, "failed to build TLS config")
		}
		if transport, err = minio.DefaultTransport(cfg.Secure); err != nil {
			return nil, errors.Wrap(err, "failed to create transport")
		}
		transport.TLSClientConfig = tlsConfig
		opts.Transport = transport
	}

	// Create the client
	if cli, err = minio.New(cfg.Endpoint, opts); err != nil {
		return nil, errors.Wrap(err, "failed to create minio client")
	}
	if cfg.Trace {
		cli.TraceOn(os.Stdout)
	}
	return cli, nil
}

// Client returns the global MinIO client or an error if it is not initialized.
func Client() (*minio.Client, error) {
	mu.RLock()
	defer mu.RUnlock()
	if client == nil {
		return nil, errors.New("minio client is not initialized")
	}
	return client, nil
}

// EnsureBucket ensures that the bucket exists, creates it if not exists.
func EnsureBucket(ctx context.Context, buckets ...string) error {
	cli, err := Client()
	if err != nil {
		return err
	}
	for _, bucket := range buckets {
		bucket = strings.TrimSpace(bucket)
		if len(bucket) == 0 {
			continue
		}
		if err := ensureBucket(ctx, cli, bucket); err != nil {
			return err
		}
	}
	return nil
}

// ensureBucket ensures that the bucket exists, creates it if not exists.
func ensureBucket(ctx context.Context, cli *minio.Client, bucket string) error {
	exists, err := cli.BucketExists(ctx, bucket)
	if err != nil {
		return errors.Wrapf(err, "failed to check bucket existence for bucket %s", bucket)
	}

	if !exists {
		region := config.App.Minio.Region
		if err = cli.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: region}); err != nil {
			return errors.Wrapf(err, "failed to create bucket %s with region %s", bucket, region)
		}
	}

	return nil
}

// Put uploads an object to MinIO and returns its basic metadata.
func Put(ctx context.Context, objectKey string, reader io.Reader, opts ...*PutOptions) (*ObjectInfo, error) {
	if objectKey == "" {
		return nil, errors.New("objectKey cannot be empty")
	}

	cli, err := Client()
	if err != nil {
		return nil, err
	}

	if reader == nil {
		return nil, errors.New("reader cannot be nil")
	}

	putOpts := minio.PutObjectOptions{}
	// set default size to -1 to let minio SDK handle automatically
	var size int64 = -1
	bucket := config.App.Minio.Bucket
	contentType := detectContentType(objectKey)

	if len(opts) > 0 && opts[0] != nil {
		opt := opts[0]
		if opt.ContentType != "" {
			putOpts.ContentType = opt.ContentType
			contentType = opt.ContentType
		} else {
			putOpts.ContentType = detectContentType(objectKey)
		}
		if opt.Metadata != nil {
			putOpts.UserMetadata = opt.Metadata
		}
		if opt.ContentEncoding != "" {
			putOpts.ContentEncoding = opt.ContentEncoding
		}
		if opt.CacheControl != "" {
			putOpts.CacheControl = opt.CacheControl
		}
		if opt.Size > 0 {
			size = opt.Size
		}
		if opt.Bucket != "" {
			bucket = opt.Bucket
		}
	} else {
		putOpts.ContentType = detectContentType(objectKey)
	}

	info, putErr := cli.PutObject(ctx, bucket, objectKey, reader, size, putOpts)
	if putErr != nil {
		return nil, errors.Wrapf(putErr, "failed to put object %s", objectKey)
	}

	return &ObjectInfo{
		Key:         info.Key,
		Size:        info.Size,
		ETag:        info.ETag,
		ContentType: contentType,
	}, nil
}

// Get downloads an object and returns a reader plus metadata.
func Get(ctx context.Context, objectKey string, opts ...*GetOptions) (io.ReadCloser, *ObjectInfo, error) {
	if objectKey == "" {
		return nil, nil, errors.New("objectKey cannot be empty")
	}

	cli, err := Client()
	if err != nil {
		return nil, nil, err
	}

	getOpts := minio.GetObjectOptions{}
	bucket := config.App.Minio.Bucket

	if len(opts) > 0 && opts[0] != nil {
		opt := opts[0]
		if len(opt.Bucket) > 0 {
			bucket = opt.Bucket
		}
		if opt.VersionID != "" {
			getOpts.VersionID = opt.VersionID
		}
		if opt.Range != "" {
			start, end, parseErr := parseRange(opt.Range)
			if parseErr != nil {
				return nil, nil, parseErr
			}
			if err = getOpts.SetRange(start, end); err != nil {
				return nil, nil, errors.Wrap(err, "invalid range")
			}
		}
	}

	obj, getErr := cli.GetObject(ctx, bucket, objectKey, getOpts)
	if getErr != nil {
		return nil, nil, errors.Wrapf(getErr, "failed to get object %s", objectKey)
	}

	// 获取对象信息
	stat, err := obj.Stat()
	if err != nil {
		obj.Close()
		return nil, nil, errors.Wrapf(err, "failed to stat object %s", objectKey)
	}

	info := &ObjectInfo{
		Key:          stat.Key,
		Size:         stat.Size,
		ContentType:  stat.ContentType,
		ETag:         stat.ETag,
		LastModified: stat.LastModified,
		Metadata:     stat.UserMetadata,
	}

	return obj, info, nil
}

// Remove deletes an object from MinIO.
func Remove(ctx context.Context, objectKey string, opts ...*RemoveOptions) error {
	if objectKey == "" {
		return errors.New("objectKey cannot be empty")
	}

	cli, err := Client()
	if err != nil {
		return err
	}

	removeOpts := minio.RemoveObjectOptions{}
	bucket := config.App.Minio.Bucket

	if len(opts) > 0 && opts[0] != nil {
		opt := opts[0]
		if len(opt.Bucket) > 0 {
			bucket = opt.Bucket
		}
		if opt.VersionID != "" {
			removeOpts.VersionID = opt.VersionID
		}
		if opt.ForceDelete {
			removeOpts.ForceDelete = opt.ForceDelete
		}
	}

	if err := cli.RemoveObject(ctx, bucket, objectKey, removeOpts); err != nil {
		return errors.Wrapf(err, "failed to delete object %s", objectKey)
	}

	return nil
}

// Exists checks whether an object exists.
func Exists(ctx context.Context, objectKey string, opts ...*ExistsOptions) (bool, error) {
	opt := minio.StatObjectOptions{}
	bucket := config.App.Minio.Bucket

	if len(opts) > 0 && opts[0] != nil {
		opt.VersionID = opts[0].VersionID
		if len(opts[0].Bucket) > 0 {
			bucket = opts[0].Bucket
		}
	}

	cli, err := Client()
	if err != nil {
		return false, err
	}

	_, err = cli.StatObject(ctx, bucket, objectKey, opt)
	if err != nil {
		errResp := minio.ToErrorResponse(err)
		if errResp.StatusCode == http.StatusNotFound {
			return false, nil
		}

		return false, errors.Wrapf(err, "failed to stat object %s", objectKey)
	}
	return true, nil
}

// Stat returns object metadata.
func Stat(ctx context.Context, objectKey string, opts ...*StatOptions) (*ObjectInfo, error) {
	if objectKey == "" {
		return nil, errors.New("objectKey cannot be empty")
	}

	cli, err := Client()
	if err != nil {
		return nil, err
	}

	statOpts := minio.StatObjectOptions{}
	bucket := config.App.Minio.Bucket

	if len(opts) > 0 && opts[0] != nil {
		opt := opts[0]
		if len(opt.Bucket) > 0 {
			bucket = opt.Bucket
		}
		if opt.VersionID != "" {
			statOpts.VersionID = opt.VersionID
		}
	}

	stat, statErr := cli.StatObject(ctx, bucket, objectKey, statOpts)
	if statErr != nil {
		return nil, errors.Wrapf(statErr, "failed to stat object %s", objectKey)
	}

	return &ObjectInfo{
		Key:          stat.Key,
		Size:         stat.Size,
		ContentType:  stat.ContentType,
		ETag:         stat.ETag,
		LastModified: stat.LastModified,
		Metadata:     stat.UserMetadata,
	}, nil
}

// PresignedGetURL generates a presigned GET URL with expiration capped at 7 days.
func PresignedGetURL(ctx context.Context, objectKey string, expires time.Duration, opts ...*GetOptions) (string, error) {
	if objectKey == "" {
		return "", errors.New("objectKey cannot be empty")
	}
	if expires <= 0 {
		expires = 1 * time.Hour
	}
	if expires > 7*24*time.Hour {
		expires = 7 * 24 * time.Hour
	}

	cli, err := Client()
	if err != nil {
		return "", err
	}

	bucket := config.App.Minio.Bucket
	if len(opts) > 0 && opts[0] != nil {
		opt := opts[0]
		if len(opt.Bucket) > 0 {
			bucket = opt.Bucket
		}
	}

	presignedURL, preErr := cli.PresignedGetObject(ctx, bucket, objectKey, expires, url.Values{})
	if preErr != nil {
		return "", errors.Wrapf(preErr, "failed to generate presigned URL for %s", objectKey)
	}

	return presignedURL.String(), nil
}

// PresignedPutURL generates a presigned PUT URL with expiration capped at 7 days.
func PresignedPutURL(ctx context.Context, objectKey string, expires time.Duration, opts ...*GetOptions) (string, error) {
	if objectKey == "" {
		return "", errors.New("objectKey cannot be empty")
	}
	if expires <= 0 {
		expires = 1 * time.Hour
	}
	if expires > 7*24*time.Hour {
		expires = 7 * 24 * time.Hour
	}

	cli, err := Client()
	if err != nil {
		return "", err
	}

	bucket := config.App.Minio.Bucket
	if len(opts) > 0 && opts[0] != nil {
		opt := opts[0]
		if len(opt.Bucket) > 0 {
			bucket = opt.Bucket
		}
	}

	presignedURL, preErr := cli.PresignedPutObject(ctx, bucket, objectKey, expires)
	if preErr != nil {
		return "", errors.Wrapf(preErr, "failed to generate presigned put URL for %s", objectKey)
	}

	return presignedURL.String(), nil
}

// List lists objects under the prefix. Errors from listing are skipped.
func List(ctx context.Context, opts ...*ListOptions) <-chan ObjectInfo {
	ch := make(chan ObjectInfo)

	cli, err := Client()
	if err != nil {
		close(ch)
		return ch
	}

	go func() {
		defer close(ch)

		listOpts := minio.ListObjectsOptions{}
		bucket := config.App.Minio.Bucket
		if len(opts) > 0 && opts[0] != nil {
			opt := opts[0]
			listOpts.Prefix = opt.Prefix
			listOpts.Recursive = opt.Recursive
			if len(opt.Bucket) > 0 {
				bucket = opt.Bucket
			}
		}

		for obj := range cli.ListObjects(ctx, bucket, listOpts) {
			if obj.Err != nil {
				logger.Minio.Error(err)
				continue
			}
			ch <- ObjectInfo{
				Key:          obj.Key,
				Size:         obj.Size,
				ContentType:  obj.ContentType,
				ETag:         obj.ETag,
				LastModified: obj.LastModified,
			}
		}
	}()

	return ch
}

// Copy copies an object within the same bucket.
func Copy(ctx context.Context, srcKey, dstKey string, opts ...*CopyOptions) (*ObjectInfo, error) {
	if srcKey == "" || dstKey == "" {
		return nil, errors.New("srcKey and dstKey cannot be empty")
	}

	cli, err := Client()
	if err != nil {
		return nil, err
	}

	bucket := config.App.Minio.Bucket
	if len(opts) > 0 && opts[0] != nil {
		bucket = opts[0].Bucket
	}

	src := minio.CopySrcOptions{
		Bucket: bucket,
		Object: srcKey,
	}

	dst := minio.CopyDestOptions{
		Bucket: bucket,
		Object: dstKey,
	}

	info, copyErr := cli.CopyObject(ctx, dst, src)
	if copyErr != nil {
		return nil, errors.Wrapf(copyErr, "failed to copy object from %s to %s", srcKey, dstKey)
	}

	return &ObjectInfo{
		Key:          info.Key,
		Size:         info.Size,
		ETag:         info.ETag,
		LastModified: info.LastModified,
	}, nil
}

// parseRange parses range string in the form "bytes=start-end".
func parseRange(r string) (int64, int64, error) {
	if !strings.HasPrefix(r, "bytes=") {
		return 0, 0, errors.New("invalid range prefix")
	}
	rangeVal := strings.TrimPrefix(r, "bytes=")
	parts := strings.Split(rangeVal, "-")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return 0, 0, errors.New("invalid range format")
	}

	start, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, errors.Wrap(err, "invalid range start")
	}
	end, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, 0, errors.Wrap(err, "invalid range end")
	}
	if end < start {
		return 0, 0, errors.New("invalid range: end before start")
	}
	return start, end, nil
}
