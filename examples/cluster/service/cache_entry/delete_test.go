package cacheentry_test

import (
	"testing"

	"cluster/internal/testsupport"
	"cluster/model"
	"cluster/pb"

	gstgrpc "github.com/hydroan/gst/grpc"
	"github.com/hydroan/gst/testutil"
	"github.com/hydroan/gst/testutil/iamtest"
	"github.com/stretchr/testify/require"
)

// TestDeleteRemovesTheEntry proves the removal path, over HTTP and over the
// DeleteCacheEntry rpc alike: the entry is gone from the store of the
// replica that answered, which is the operation the other replicas must
// apply too — a replica that missed it keeps serving the entry until the
// ttl runs out, which the deployment scenarios in the README check.
func TestDeleteRemovesTheEntry(t *testing.T) {
	ctx := gstgrpc.WithBearer(t.Context(), iamtest.GRPCSession(t, testsupport.Root))
	t.Run("over HTTP", func(t *testing.T) {
		cli := iamtest.Login(t, testsupport.Root).Client

		_, err := cli.Post[model.CacheEntryCreateRsp](ctx, "/api/cache_entries", model.CacheEntryReq{Key: "deleted-key", Value: "deleted-value"})
		require.NoError(t, err)

		removed, err := cli.Delete[model.CacheEntryDeleteRsp](ctx, "/api/cache_entries/deleted-key", nil)
		require.NoError(t, err)
		require.Equal(t, "deleted-key", removed.Key)

		read, err := cli.Get[model.CacheEntryGetRsp](ctx, "/api/cache_entries/deleted-key")
		require.NoError(t, err)
		require.False(t, read.Found, "the entry is gone from the store of the replica that removed it")
	})

	t.Run("over gRPC", func(t *testing.T) {
		entries := pb.NewCacheEntryServiceClient(testutil.DialGRPC(t))

		_, err := entries.CreateCacheEntry(ctx, &pb.CreateCacheEntryRequest{Payload: &pb.CacheEntryReq{Key: "deleted-over-grpc", Value: "deleted-value"}})
		require.NoError(t, err)

		removed, err := entries.DeleteCacheEntry(ctx, &pb.DeleteCacheEntryRequest{Id: "deleted-over-grpc"})
		require.NoError(t, err)
		require.Equal(t, "deleted-over-grpc", removed.GetResult().GetKey())

		read, err := entries.GetCacheEntry(ctx, &pb.GetCacheEntryRequest{Id: "deleted-over-grpc"})
		require.NoError(t, err)
		require.False(t, read.GetResult().GetFound(), "the entry is gone from the store of the replica that removed it")
	})
}
