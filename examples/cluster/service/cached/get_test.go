package cached_test

import (
	"testing"

	"cluster/model"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
)

// TestGetAnswersWhatThisReplicaHolds proves the read path answers from the
// replica's own store and nothing else: a key this replica never received
// reads as missing instead of being fetched from a peer, which is what makes
// a missed event visible from outside.
func TestGetAnswersWhatThisReplicaHolds(t *testing.T) {
	cli, err := client.New(testutil.BaseURL())
	require.NoError(t, err)

	missing, err := cli.Get[model.CachedRsp]("/api/caches/never-written-key")
	require.NoError(t, err)
	require.False(t, missing.Found, "a key this replica never received reads as missing")
	require.Empty(t, missing.Value)

	_, err = cli.Post[model.CachedRsp]("/api/caches", model.CachedReq{Key: "read-key", Value: "read-value"})
	require.NoError(t, err)

	read, err := cli.Get[model.CachedRsp]("/api/caches/read-key")
	require.NoError(t, err)
	require.True(t, read.Found)
	require.Equal(t, "read-value", read.Value)
}
