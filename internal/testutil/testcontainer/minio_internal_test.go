package testcontainer

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/stretchr/testify/require"
)

func TestSetupMinio(t *testing.T) {
	isolateEnv(t, config.MINIO_ENDPOINT, config.MINIO_ACCESS_KEY, config.MINIO_SECRET_KEY, config.MINIO_BUCKET, config.MINIO_ENABLED)

	release, err := SetupMinio()
	require.NoError(t, err)
	released := false
	t.Cleanup(func() {
		if !released {
			require.NoError(t, release())
		}
	})

	endpoint := os.Getenv(config.MINIO_ENDPOINT)
	bucket := os.Getenv(config.MINIO_BUCKET)
	require.NotEmpty(t, endpoint)
	require.NotEmpty(t, bucket)
	require.Equal(t, "true", os.Getenv(config.MINIO_ENABLED))

	// A put-get round trip in the bucket the setup assigned is what proves
	// the framework can use it; a bare dial would pass on a server that
	// rejects every request.
	cli, err := minio.New(endpoint, &minio.Options{
		Creds: credentials.NewStaticV4(os.Getenv(config.MINIO_ACCESS_KEY), os.Getenv(config.MINIO_SECRET_KEY), ""),
	})
	require.NoError(t, err)
	ctx := context.Background()
	putNote(t, cli, bucket)
	obj, err := cli.GetObject(ctx, bucket, "note.txt", minio.GetObjectOptions{})
	require.NoError(t, err)
	defer obj.Close()
	content, err := io.ReadAll(obj)
	require.NoError(t, err)
	require.Equal(t, "value", string(content))

	// A dedicated container goes away with every bucket on release; the
	// cleanup below is what the shared container depends on.
	if dedicatedContainersRequested() {
		return
	}

	// A bucket named after the default one belongs to this binary as well.
	extra := bucket + "-extra"
	require.NoError(t, cli.MakeBucket(ctx, extra, minio.MakeBucketOptions{}))
	putNote(t, cli, extra)
	// A bucket of a binary that died without releasing, and one this setup
	// did not name.
	abandoned := fmt.Sprintf("gst-test-999999999-%d-ffff", time.Now().Unix())
	require.NoError(t, cli.MakeBucket(ctx, abandoned, minio.MakeBucketOptions{}))
	putNote(t, cli, abandoned)
	foreign := "gst-foreign-" + randomHex(t)
	require.NoError(t, cli.MakeBucket(ctx, foreign, minio.MakeBucketOptions{}))
	t.Cleanup(func() {
		require.NoError(t, cli.RemoveBucketWithOptions(ctx, foreign, minio.RemoveBucketOptions{ForceDelete: true}))
	})

	// Releasing removes the buckets of this binary, objects included, and
	// leaves every other bucket alone.
	released = true
	require.NoError(t, release())
	requireBucketExists(t, cli, bucket, false)
	requireBucketExists(t, cli, extra, false)
	requireBucketExists(t, cli, abandoned, true)

	// The next binary to set up removes the abandoned bucket; the foreign
	// one is never touched.
	next, err := SetupMinio()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, next()) })
	requireBucketExists(t, cli, abandoned, false)
	requireBucketExists(t, cli, foreign, true)
}

func TestSharedBucketAbandoned(t *testing.T) {
	// A fresh bucket of this very process is owned, and so is one named
	// after it.
	own := strings.ReplaceAll(newSharedDatabaseName(), "_", "-")
	require.False(t, sharedBucketAbandoned(own))
	require.False(t, sharedBucketAbandoned(own+"-list"))

	// A dead pid marks the bucket and the ones named after it as leftovers.
	dead := fmt.Sprintf("gst-test-999999999-%d-ffff", time.Now().Unix())
	require.True(t, sharedBucketAbandoned(dead))
	require.True(t, sharedBucketAbandoned(dead+"-list"))

	// A bucket this setup did not name is never ours to remove.
	for _, name := range []string{"sample", "test-bucket", "gst-test", "gst-test-1-abc-ffff", "other-1-123-ffff"} {
		require.False(t, sharedBucketAbandoned(name), "name %q", name)
	}
}

// putNote stores a small object in bucket, so removing the bucket has to
// remove its objects too.
func putNote(t *testing.T, cli *minio.Client, bucket string) {
	t.Helper()

	_, err := cli.PutObject(context.Background(), bucket, "note.txt", strings.NewReader("value"), int64(len("value")), minio.PutObjectOptions{})
	require.NoError(t, err)
}

// requireBucketExists fails the test unless bucket exists exactly when want
// says so.
func requireBucketExists(t *testing.T, cli *minio.Client, bucket string, want bool) {
	t.Helper()

	exists, err := cli.BucketExists(context.Background(), bucket)
	require.NoError(t, err)
	require.Equal(t, want, exists, "bucket %s", bucket)
}

// randomHex returns a short random hex string for names that must not collide
// across runs.
func randomHex(t *testing.T) string {
	t.Helper()

	b := make([]byte, 4)
	_, err := rand.Read(b)
	require.NoError(t, err)
	return hex.EncodeToString(b)
}
