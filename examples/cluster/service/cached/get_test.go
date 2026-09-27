package cached_test

import (
	"testing"

	"cluster/internal/testsupport"
	"cluster/model"
	"cluster/pb"

	"github.com/stretchr/testify/require"
)

// TestGetAnswersWhatThisReplicaHolds proves the read path answers from the
// replica's own store and nothing else, over HTTP and over the GetCached rpc
// alike: a key this replica never received reads as missing instead of being
// fetched from a peer, which is what makes a missed event visible from
// outside.
func TestGetAnswersWhatThisReplicaHolds(t *testing.T) {
	ctx := testsupport.Authorized(t.Context(), testsupport.Session(t))
	t.Run("over HTTP", func(t *testing.T) {
		cli := testsupport.Login(t).Client

		missing, err := cli.Get[model.CachedGetRsp](ctx, "/api/caches/never-written-key")
		require.NoError(t, err)
		require.False(t, missing.Found, "a key this replica never received reads as missing")
		require.Empty(t, missing.Value)

		_, err = cli.Post[model.CachedCreateRsp](ctx, "/api/caches", model.CachedReq{Key: "read-key", Value: "read-value"})
		require.NoError(t, err)

		read, err := cli.Get[model.CachedGetRsp](ctx, "/api/caches/read-key")
		require.NoError(t, err)
		require.True(t, read.Found)
		require.Equal(t, "read-value", read.Value)
	})

	t.Run("over gRPC", func(t *testing.T) {
		caches := pb.NewCachedServiceClient(testsupport.Dial(t))

		missing, err := caches.GetCached(ctx, &pb.GetCachedRequest{Id: "never-written-over-grpc"})
		require.NoError(t, err)
		require.False(t, missing.GetResult().GetFound(), "a key this replica never received reads as missing")

		_, err = caches.CreateCached(ctx, &pb.CreateCachedRequest{Payload: &pb.CachedReq{Key: "read-over-grpc", Value: "read-value"}})
		require.NoError(t, err)

		read, err := caches.GetCached(ctx, &pb.GetCachedRequest{Id: "read-over-grpc"})
		require.NoError(t, err)
		require.True(t, read.GetResult().GetFound())
		require.Equal(t, "read-value", read.GetResult().GetValue())
	})
}
