package minio_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger/zap"
	"github.com/hydroan/gst/provider/minio"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	os.Setenv(config.MINIO_ENABLED, "true")
	os.Setenv(config.MINIO_BUCKET, "test-bucket")

	if err := config.Init(); err != nil {
		panic(err)
	}
	if err := zap.Init(); err != nil {
		panic(err)
	}
	if err := minio.Init(); err != nil {
		fmt.Println("minio not available, skipping tests:", err)
		os.Exit(0)
	}

	if err := minio.EnsureBucket(context.Background(), "bucket-exists", "bucket-presign", "bucket-list"); err != nil {
		panic(err)
	}

	os.Exit(m.Run())
}

func TestEnsureBucket(t *testing.T) {
	require.NoError(t, minio.EnsureBucket(context.TODO(), "bucket1", "bucket2", "bucket3"))

	cli, err := minio.Client()
	require.NoError(t, err)

	exists1, err1 := cli.BucketExists(context.TODO(), "bucket1")
	exists2, err2 := cli.BucketExists(context.TODO(), "bucket2")
	exists3, err3 := cli.BucketExists(context.TODO(), "bucket3")
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
		info, err := minio.Put(context.TODO(), "hello.txt", strings.NewReader("hello world"), &minio.PutOptions{Bucket: "bucket1"})
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
		_, err := minio.Put(context.TODO(), "hello.txt", strings.NewReader("hello world"), &minio.PutOptions{Bucket: "bucket1"})
		require.NoError(t, err)
		data, info, err := minio.Get(context.TODO(), "hello.txt", &minio.GetOptions{Bucket: "bucket1"})
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
	bucket := "bucket-exists"
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
	bucket := "bucket-presign"
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
	bucket := "bucket-list"
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
