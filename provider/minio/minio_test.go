package minio_test

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/lifecycle"
	"github.com/hydroan/gst/internal/testutil/testcontainer"
	"github.com/hydroan/gst/logger/zap"
	"github.com/hydroan/gst/provider/minio"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

// runTests prepares the minio server the tests store objects in. os.Exit in
// TestMain would skip the deferred release, hence the wrapper.
func runTests(m *testing.M) int {
	// Before config.Init: the container publishes its address and credentials
	// through the environment, which is what config reads.
	release, err := testcontainer.SetupMinio()
	if err != nil {
		panic(err)
	}
	defer func() { _ = release() }()

	// File mode keeps the logs out of the test output, where every stream
	// would otherwise go with stdout the default, once the global stream has a
	// file of its own and the console mirror is off: in file mode the global
	// stream still writes to stdout when it names no file, and the mirror
	// copies it there when it names one. A log directory of its own keeps the
	// files out of the package source tree: log files written there change
	// with every run, which go's test cache takes for changed source in every
	// test that reads or lists that tree.
	logDir, err := os.MkdirTemp("", "gst_logs_")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(logDir) }()
	os.Setenv(config.LOGGER_OUTPUT, string(config.LoggerOutputFile))
	os.Setenv(config.LOGGER_DIR, logDir)
	os.Setenv(config.LOGGER_FILE, "global.log")
	os.Setenv(config.LOGGER_CONSOLE, "false")

	if err := config.Init(); err != nil {
		panic(err)
	}
	if err := zap.Init(); err != nil {
		panic(err)
	}
	// Bring the compiled-in providers up through the registry — the same
	// entry the provider stage takes at bootstrap — instead of a test-only
	// export of the unexported lifecycle.
	for _, p := range lifecycle.Components(lifecycle.StageProvider) {
		if err := p.Start(context.Background()); err != nil {
			panic(err)
		}
	}

	if err := minio.EnsureBucket(context.Background(), bucketNamed("exists"), bucketNamed("presign"), bucketNamed("list")); err != nil {
		panic(err)
	}

	return m.Run()
}

func TestEnsureBucket(t *testing.T) {
	require.NoError(t, minio.EnsureBucket(context.TODO(), bucketNamed("1"), bucketNamed("2"), bucketNamed("3")))

	cli, err := minio.Client()
	require.NoError(t, err)

	exists1, err1 := cli.BucketExists(context.TODO(), bucketNamed("1"))
	exists2, err2 := cli.BucketExists(context.TODO(), bucketNamed("2"))
	exists3, err3 := cli.BucketExists(context.TODO(), bucketNamed("3"))
	require.NoError(t, err1)
	require.NoError(t, err2)
	require.NoError(t, err3)
	require.True(t, exists1)
	require.True(t, exists2)
	require.True(t, exists3)
}

func TestPut(t *testing.T) {
	t.Run("default bucket", func(t *testing.T) {
		info, err := minio.Put(context.TODO(), "hello.txt", strings.NewReader("hello world"))
		require.NoError(t, err)

		require.NotNil(t, info)
		require.Equal(t, "hello.txt", info.Key)
		require.Equal(t, int64(11), info.Size)
		require.Equal(t, "text/plain", info.ContentType)
		require.NotEmpty(t, info.ETag)
	})
	t.Run("named bucket", func(t *testing.T) {
		info, err := minio.Put(context.TODO(), "hello.txt", strings.NewReader("hello world"), &minio.PutOptions{Bucket: bucketNamed("1")})
		require.NoError(t, err)

		require.NotNil(t, info)
		require.Equal(t, "hello.txt", info.Key)
		require.Equal(t, int64(11), info.Size)
		require.Equal(t, "text/plain", info.ContentType)
		require.NotEmpty(t, info.ETag)
	})
}

func TestGet(t *testing.T) {
	t.Run("default bucket", func(t *testing.T) {
		_, err := minio.Put(context.TODO(), "hello.txt", strings.NewReader("hello world"))
		require.NoError(t, err)
		data, info, err := minio.Get(context.TODO(), "hello.txt")
		require.NoError(t, err)

		content, err := io.ReadAll(data)
		require.NoError(t, err)

		require.Equal(t, "hello world", string(content))
		require.Equal(t, "hello.txt", info.Key)
		require.Equal(t, int64(11), info.Size)
		require.Equal(t, "text/plain", info.ContentType)
		require.NotEmpty(t, info.ETag)
		require.NotEmpty(t, info.LastModified)
	})

	t.Run("named bucket", func(t *testing.T) {
		_, err := minio.Put(context.TODO(), "hello.txt", strings.NewReader("hello world"), &minio.PutOptions{Bucket: bucketNamed("1")})
		require.NoError(t, err)
		data, info, err := minio.Get(context.TODO(), "hello.txt", &minio.GetOptions{Bucket: bucketNamed("1")})
		require.NoError(t, err)

		content, err := io.ReadAll(data)
		require.NoError(t, err)

		require.Equal(t, "hello world", string(content))
		require.Equal(t, "hello.txt", info.Key)
		require.Equal(t, int64(11), info.Size)
		require.Equal(t, "text/plain", info.ContentType)
		require.NotEmpty(t, info.ETag)
		require.NotEmpty(t, info.LastModified)
	})
}

func TestRemove(t *testing.T) {
	t.Run("default bucket", func(t *testing.T) {
		_, err := minio.Put(context.TODO(), "hello.txt", strings.NewReader("hello world"))
		require.NoError(t, err)

		err = minio.Remove(context.TODO(), "hello.txt")
		require.NoError(t, err)

		_, info, err := minio.Get(context.TODO(), "hello.txt")
		require.Error(t, err)
		require.Nil(t, info)
	})

	t.Run("named bucket", func(t *testing.T) {
		_, err := minio.Put(context.TODO(), "hello.txt", strings.NewReader("hello world"))
		require.NoError(t, err)

		err = minio.Remove(context.TODO(), "hello.txt")
		require.NoError(t, err)

		_, info, err := minio.Get(context.TODO(), "hello.txt")
		require.Error(t, err)
		require.Nil(t, info)
	})
}

func TestExistsAndStat(t *testing.T) {
	bucket := bucketNamed("exists")
	_, err := minio.Put(context.TODO(), "exists.txt", strings.NewReader("exists"), &minio.PutOptions{Bucket: bucket})
	require.NoError(t, err)

	exists, err := minio.Exists(context.TODO(), "exists.txt", &minio.ExistsOptions{Bucket: bucket})
	require.NoError(t, err)
	require.True(t, exists)

	info, err := minio.Stat(context.TODO(), "exists.txt", &minio.StatOptions{Bucket: bucket})
	require.NoError(t, err)
	require.Equal(t, "exists.txt", info.Key)
	require.Equal(t, int64(6), info.Size)

	err = minio.Remove(context.TODO(), "exists.txt", &minio.RemoveOptions{Bucket: bucket})
	require.NoError(t, err)

	exists, err = minio.Exists(context.TODO(), "exists.txt", &minio.ExistsOptions{Bucket: bucket})
	require.NoError(t, err)
	require.False(t, exists)
}

func TestPresignedURL(t *testing.T) {
	bucket := bucketNamed("presign")
	_, err := minio.Put(context.TODO(), "presign.txt", strings.NewReader("presign"), &minio.PutOptions{Bucket: bucket})
	require.NoError(t, err)
	defer func() {
		_ = minio.Remove(context.TODO(), "presign.txt", &minio.RemoveOptions{Bucket: bucket})
	}()

	url, err := minio.PresignedGetURL(context.TODO(), "presign.txt", 10*time.Minute, &minio.GetOptions{Bucket: bucket})
	require.NoError(t, err)
	require.Contains(t, url, "presign.txt")

	url, err = minio.PresignedPutURL(context.TODO(), "presign-put.txt", 10*time.Minute, &minio.GetOptions{Bucket: bucket})
	require.NoError(t, err)
	require.Contains(t, url, "presign-put.txt")
}

func TestListAndCopy(t *testing.T) {
	bucket := bucketNamed("list")
	_, err := minio.Put(context.TODO(), "list/a.txt", strings.NewReader("a"), &minio.PutOptions{Bucket: bucket})
	require.NoError(t, err)
	_, err = minio.Put(context.TODO(), "list/b.txt", strings.NewReader("b"), &minio.PutOptions{Bucket: bucket})
	require.NoError(t, err)
	defer func() {
		_ = minio.Remove(context.TODO(), "list/a.txt", &minio.RemoveOptions{Bucket: bucket})
		_ = minio.Remove(context.TODO(), "list/b.txt", &minio.RemoveOptions{Bucket: bucket})
		_ = minio.Remove(context.TODO(), "list/copy.txt", &minio.RemoveOptions{Bucket: bucket})
	}()

	keys := make([]string, 0, 2)
	for obj := range minio.List(context.TODO(), &minio.ListOptions{Prefix: "list/", Recursive: true, Bucket: bucket}) {
		keys = append(keys, obj.Key)
	}
	require.ElementsMatch(t, []string{"list/a.txt", "list/b.txt"}, keys)

	_, err = minio.Copy(context.TODO(), "list/a.txt", "list/copy.txt", &minio.CopyOptions{Bucket: bucket})
	require.NoError(t, err)

	exists, err := minio.Exists(context.TODO(), "list/copy.txt", &minio.ExistsOptions{Bucket: bucket})
	require.NoError(t, err)
	require.True(t, exists)
}

// TestCopyWithEmptyOptionsUsesTheDefaultBucket copies within the configured
// bucket through options that name no bucket: an empty Bucket stands for the
// configured one, as it does for every other operation.
func TestCopyWithEmptyOptionsUsesTheDefaultBucket(t *testing.T) {
	_, err := minio.Put(context.TODO(), "copy/source.txt", strings.NewReader("source"))
	require.NoError(t, err)
	defer func() {
		_ = minio.Remove(context.TODO(), "copy/source.txt")
		_ = minio.Remove(context.TODO(), "copy/target.txt")
	}()

	_, err = minio.Copy(context.TODO(), "copy/source.txt", "copy/target.txt", &minio.CopyOptions{})
	require.NoError(t, err)

	exists, err := minio.Exists(context.TODO(), "copy/target.txt")
	require.NoError(t, err)
	require.True(t, exists)
}

// TestDefaultBucket pins the bucket an operation naming none uses: the first
// bucket the setting lists, comma separated as the configuration allows, and
// none when the setting lists no bucket at all.
func TestDefaultBucket(t *testing.T) {
	t.Run("the first bucket listed", func(t *testing.T) {
		// The second bucket stays reachable by name.
		first, second := config.App.Minio.Bucket, bucketNamed("second")
		require.NoError(t, minio.EnsureBucket(context.TODO(), second))
		config.App.Minio.Bucket = first + ", " + second
		t.Cleanup(func() { config.App.Minio.Bucket = first })

		_, err := minio.Put(context.TODO(), "default/note.txt", strings.NewReader("note"))
		require.NoError(t, err)
		defer func() { _ = minio.Remove(context.TODO(), "default/note.txt") }()

		exists, err := minio.Exists(context.TODO(), "default/note.txt")
		require.NoError(t, err)
		require.True(t, exists)
		reader, _, err := minio.Get(context.TODO(), "default/note.txt", &minio.GetOptions{Bucket: first})
		require.NoError(t, err)
		defer reader.Close()
		content, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.Equal(t, "note", string(content))
		exists, err = minio.Exists(context.TODO(), "default/note.txt", &minio.ExistsOptions{Bucket: second})
		require.NoError(t, err)
		require.False(t, exists)
	})

	t.Run("none when the setting lists none", func(t *testing.T) {
		// Separators alone list no bucket, so an operation naming none has
		// nowhere to write and fails.
		first := config.App.Minio.Bucket
		config.App.Minio.Bucket = " , "
		t.Cleanup(func() { config.App.Minio.Bucket = first })

		_, err := minio.Put(context.TODO(), "default/note.txt", strings.NewReader("note"))
		require.Error(t, err)
	})
}

// bucketNamed returns the name of a further bucket of this test binary: the
// default bucket SetupMinio assigned followed by suffix, which is what makes
// the release remove it together with the default one.
func bucketNamed(suffix string) string {
	return config.App.Minio.Bucket + "-" + suffix
}
