package cached_test

import (
	"testing"

	"cluster/internal/testsupport"
	"cluster/model"
	"cluster/pb"

	"github.com/stretchr/testify/require"
)

// TestDeleteRemovesTheEntry proves the removal path, over HTTP and over the
// DeleteCached rpc alike: the entry is gone from the store of the replica
// that answered, which is the operation the other replicas must apply too — a
// replica that missed it keeps serving the entry until the ttl runs out,
// which the deployment scenarios in the README check.
func TestDeleteRemovesTheEntry(t *testing.T) {
	ctx := testsupport.Authorized(t.Context(), testsupport.Session(t))
	t.Run("over HTTP", func(t *testing.T) {
		cli := testsupport.Login(t).Client

		_, err := cli.Post[model.CachedCreateRsp](ctx, "/api/caches", model.CachedReq{Key: "deleted-key", Value: "deleted-value"})
		require.NoError(t, err)

		removed, err := cli.Delete[model.CachedDeleteRsp](ctx, "/api/caches/deleted-key", nil)
		require.NoError(t, err)
		require.Equal(t, "deleted-key", removed.Key)

		read, err := cli.Get[model.CachedGetRsp](ctx, "/api/caches/deleted-key")
		require.NoError(t, err)
		require.False(t, read.Found, "the entry is gone from the store of the replica that removed it")
	})

	t.Run("over gRPC", func(t *testing.T) {
		caches := pb.NewCachedServiceClient(testsupport.Dial(t))

		_, err := caches.CreateCached(ctx, &pb.CreateCachedRequest{Payload: &pb.CachedReq{Key: "deleted-over-grpc", Value: "deleted-value"}})
		require.NoError(t, err)

		removed, err := caches.DeleteCached(ctx, &pb.DeleteCachedRequest{Id: "deleted-over-grpc"})
		require.NoError(t, err)
		require.Equal(t, "deleted-over-grpc", removed.GetResult().GetKey())

		read, err := caches.GetCached(ctx, &pb.GetCachedRequest{Id: "deleted-over-grpc"})
		require.NoError(t, err)
		require.False(t, read.GetResult().GetFound(), "the entry is gone from the store of the replica that removed it")
	})
}
