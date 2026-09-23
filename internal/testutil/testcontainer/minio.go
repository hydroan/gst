package testcontainer

import (
	"context"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	miniogo "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/testcontainers/testcontainers-go/modules/minio"
)

const minioImage = "quay.io/minio/minio:RELEASE.2025-09-07T16-13-09Z"

// SetupMinio prepares a minio bucket and points the framework at it as the
// default bucket, returning the function that releases it. A test that needs
// further buckets names them after the default one, such as <bucket>-list:
// they belong to the test binary as well, and the release removes them with
// it.
//
// By default the bucket lives in the shared minio container, see shared.go:
// every binary gets a bucket named like a shared database,
// gst-test-<pid>-<unix>-<hex>, and releasing removes its buckets, objects
// included, but never the container.
func SetupMinio() (func() error, error) {
	if dedicatedContainersRequested() {
		return setupDedicatedMinio()
	}
	return setupSharedMinio()
}

// setupDedicatedMinio starts a minio container of its own and points the
// framework at it. The returned function terminates that container.
func setupDedicatedMinio() (func() error, error) {
	muteContainerLog()
	ctx := context.Background()

	c, err := minio.Run(ctx, minioImage)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start minio container")
	}
	terminate := func() error { return c.Terminate(ctx) }

	addr, err := c.ConnectionString(ctx)
	if err != nil {
		return nil, errors.CombineErrors(errors.Wrap(err, "failed to resolve the minio endpoint"), terminate())
	}
	client, err := newMinioClient(addr, c.Username, c.Password)
	if err != nil {
		return nil, errors.CombineErrors(err, terminate())
	}
	bucket, err := createSharedBucket(ctx, client)
	if err != nil {
		return nil, errors.CombineErrors(err, terminate())
	}

	applyMinioConfig(addr, c.Username, c.Password, bucket)
	return terminate, nil
}

// setupSharedMinio attaches to the shared minio container, creating it when
// it is not running yet, and creates the bucket of this test binary. The
// returned function removes the buckets of the binary; the container stays.
func setupSharedMinio() (func() error, error) {
	muteContainerLog()
	ctx := context.Background()
	containerName := sharedContainerName(minioImage)

	var (
		addr, username, password string
		client                   *miniogo.Client
		bucket                   string
	)
	err := withSharedContainerLock(containerName, func() error {
		c, err := minio.Run(ctx, minioImage, reuseAcrossRuns(containerName))
		if err != nil {
			return errors.Wrap(err, "failed to start the shared minio container")
		}
		if addr, err = c.ConnectionString(ctx); err != nil {
			return errors.Wrap(err, "failed to resolve the minio endpoint")
		}
		username, password = c.Username, c.Password
		if client, err = newMinioClient(addr, username, password); err != nil {
			return err
		}
		if err = removeSharedBuckets(ctx, client, sharedBucketAbandoned); err != nil {
			return err
		}
		bucket, err = createSharedBucket(ctx, client)
		return err
	})
	if err != nil {
		return nil, err
	}

	applyMinioConfig(addr, username, password, bucket)
	release := func() error {
		return removeSharedBuckets(ctx, client, func(name string) bool {
			return name == bucket || strings.HasPrefix(name, bucket+"-")
		})
	}
	return release, nil
}

// newMinioClient connects to the minio server at addr with the root
// credentials the container was started with.
func newMinioClient(addr, username, password string) (*miniogo.Client, error) {
	client, err := miniogo.New(addr, &miniogo.Options{Creds: credentials.NewStaticV4(username, password, "")})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create the minio admin client")
	}
	return client, nil
}

// createSharedBucket creates the bucket of this test binary and returns its
// name, gst-test-<pid>-<unix>-<hex>: a shared database name with hyphens,
// since a bucket name cannot hold an underscore.
func createSharedBucket(ctx context.Context, client *miniogo.Client) (string, error) {
	bucket := strings.ReplaceAll(newSharedDatabaseName(), "_", "-")
	if err := client.MakeBucket(ctx, bucket, miniogo.MakeBucketOptions{}); err != nil {
		return "", errors.Wrapf(err, "failed to create the test bucket %s", bucket)
	}
	return bucket, nil
}

// sharedBucketAbandoned reports whether a bucket belongs to a test binary that
// is gone: the claim its name starts with, gst-test-<pid>-<unix>-<hex> as in
// gst-test-4242-1767323045-9f86d081-list, is judged the way a shared database
// name is, see sharedDatabaseAbandoned. Buckets this setup did not name are
// never abandoned.
func sharedBucketAbandoned(name string) bool {
	parts := strings.SplitN(name, "-", 6)
	if len(parts) < 5 {
		return false
	}
	return sharedDatabaseAbandoned(strings.Join(parts[:5], "_"))
}

// removeSharedBuckets removes every bucket whose name match accepts, objects
// included.
func removeSharedBuckets(ctx context.Context, client *miniogo.Client, match func(string) bool) error {
	buckets, err := client.ListBuckets(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list the buckets of the minio container")
	}
	for _, b := range buckets {
		if !match(b.Name) {
			continue
		}
		if err := client.RemoveBucketWithOptions(ctx, b.Name, miniogo.RemoveBucketOptions{ForceDelete: true}); err != nil {
			return errors.Wrapf(err, "failed to remove the test bucket %s", b.Name)
		}
	}
	return nil
}

// applyMinioConfig points the framework at the bucket of this test binary.
func applyMinioConfig(addr, username, password, bucket string) {
	ApplyConfigToEnv(config.Minio{
		Endpoint:  addr,
		AccessKey: username,
		SecretKey: password,
		Bucket:    bucket,
		Enabled:   true,
	})
	reportServiceReady("minio", addr+"/"+bucket)
}
