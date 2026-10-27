package minio

import (
	"context"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

// TestStartCreatesTheConfiguredBuckets brings the provider up again under a
// setting listing two buckets the store does not hold yet: start creates both.
// They are named after the default bucket SetupMinio assigned, which is what
// makes the release remove them together with it.
func TestStartCreatesTheConfiguredBuckets(t *testing.T) {
	setting := config.App.Minio.Bucket
	first, second := setting+"-start-1", setting+"-start-2"
	mu.Lock()
	running := client
	client = nil
	mu.Unlock()
	config.App.Minio.Bucket = first + ", " + second
	t.Cleanup(func() {
		mu.Lock()
		client = running
		mu.Unlock()
		config.App.Minio.Bucket = setting
	})

	require.NoError(t, start(context.TODO()))
	for _, bucket := range []string{first, second} {
		exists, err := client.BucketExists(context.TODO(), bucket)
		require.NoError(t, err)
		require.True(t, exists, bucket)
	}
}
