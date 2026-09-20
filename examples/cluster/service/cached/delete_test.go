package cached_test

import (
	"testing"

	"cluster/model"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
)

// TestDeleteRemovesTheEntry proves the removal path: the entry is gone from
// the store of the replica that answered, which is the operation the other
// replicas must apply too — a replica that missed it keeps serving the entry
// until the ttl runs out, which the deployment scenarios in the README check.
func TestDeleteRemovesTheEntry(t *testing.T) {
	cli, err := client.New(testutil.BaseURL())
	require.NoError(t, err)

	_, err = cli.Post[model.CachedRsp]("/api/caches", model.CachedReq{Key: "deleted-key", Value: "deleted-value"})
	require.NoError(t, err)

	removed, err := cli.Delete[model.CachedRsp]("/api/caches/deleted-key", nil)
	require.NoError(t, err)
	require.Equal(t, "deleted-key", removed.Key)

	read, err := cli.Get[model.CachedRsp]("/api/caches/deleted-key")
	require.NoError(t, err)
	require.False(t, read.Found, "the entry is gone from the store of the replica that removed it")
}
