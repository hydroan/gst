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

// TestGetAnswersWhatThisReplicaHolds proves the read path answers from the
// replica's own store and nothing else, over HTTP and over the
// GetCacheEntry rpc alike: a key this replica never received reads as
// missing instead of being fetched from a peer, which is what makes a
// missed event visible from outside.
func TestGetAnswersWhatThisReplicaHolds(t *testing.T) {
	ctx := gstgrpc.WithBearer(t.Context(), iamtest.GRPCSession(t, testsupport.Root))
	t.Run("over HTTP", func(t *testing.T) {
		cli := iamtest.Login(t, testsupport.Root).Client

		missing, err := cli.Get[model.CacheEntryGetRsp](ctx, "/api/cache_entries/never-written-key")
		require.NoError(t, err)
		require.False(t, missing.Found, "a key this replica never received reads as missing")
		require.Empty(t, missing.Value)

		_, err = cli.Post[model.CacheEntryCreateRsp](ctx, "/api/cache_entries", model.CacheEntryReq{Key: "read-key", Value: "read-value"})
		require.NoError(t, err)

		read, err := cli.Get[model.CacheEntryGetRsp](ctx, "/api/cache_entries/read-key")
		require.NoError(t, err)
		require.True(t, read.Found)
		require.Equal(t, "read-value", read.Value)
	})

	t.Run("over gRPC", func(t *testing.T) {
		entries := pb.NewCacheEntryServiceClient(testutil.DialGRPC(t))

		missing, err := entries.GetCacheEntry(ctx, &pb.GetCacheEntryRequest{Id: "never-written-over-grpc"})
		require.NoError(t, err)
		require.False(t, missing.GetResult().GetFound(), "a key this replica never received reads as missing")

		_, err = entries.CreateCacheEntry(ctx, &pb.CreateCacheEntryRequest{Payload: &pb.CacheEntryReq{Key: "read-over-grpc", Value: "read-value"}})
		require.NoError(t, err)

		read, err := entries.GetCacheEntry(ctx, &pb.GetCacheEntryRequest{Id: "read-over-grpc"})
		require.NoError(t, err)
		require.True(t, read.GetResult().GetFound())
		require.Equal(t, "read-value", read.GetResult().GetValue())
	})
}
