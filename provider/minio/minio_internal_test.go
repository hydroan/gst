package minio

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
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

// TestParseRangeReadsBytesStartEnd pins the byte range an option names:
// "bytes=start-end" with both ends numbers and the end not before the start
// is read, and a range without the prefix, without both ends, with an end
// that is no number or that ends before it starts is refused.
func TestParseRangeReadsBytesStartEnd(t *testing.T) {
	start, end, err := parseRange("bytes=0-99")
	require.NoError(t, err)
	require.Equal(t, int64(0), start)
	require.Equal(t, int64(99), end)

	start, end, err = parseRange("bytes=7-7")
	require.NoError(t, err)
	require.Equal(t, int64(7), start)
	require.Equal(t, int64(7), end)

	for _, tt := range []struct {
		in   string
		want string
	}{
		{"0-99", "invalid range prefix"},
		{"bytes=0", "invalid range format"},
		{"bytes=-5", "invalid range format"},
		{"bytes=a-1", "invalid range start"},
		{"bytes=1-b", "invalid range end"},
		{"bytes=9-1", "invalid range: end before start"},
	} {
		_, _, err := parseRange(tt.in)
		require.ErrorContains(t, err, tt.want, tt.in)
	}
}

// TestObjectNotFoundErrorReadsAsItsCauseAndIsTheSentinel pins the error the
// store's 404 becomes: it reads as the cause, unwraps to it, and is
// ErrObjectNotFound to errors.Is.
func TestObjectNotFoundErrorReadsAsItsCauseAndIsTheSentinel(t *testing.T) {
	cause := errors.New("the specified key does not exist")
	err := objectNotFoundError{cause: cause}
	require.Equal(t, cause.Error(), err.Error())
	require.Equal(t, cause, err.Unwrap())
	require.ErrorIs(t, err, ErrObjectNotFound)
	require.NotErrorIs(t, errors.New("other"), ErrObjectNotFound)
}
