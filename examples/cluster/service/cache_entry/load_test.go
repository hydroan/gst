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

// TestLoad covers the Stream action on /api/cache_entries/load, served by
// Load in load.go: a client stream of entries is cached on the replica that
// took it, which answers the count once the client has closed its side, and
// the entries read back from that replica's store. Over gRPC it is the
// LoadCacheEntry rpc, the session traveling as a bearer token; over HTTP it
// is a WebSocket connection, the session traveling in the cookie the login
// set.
func TestLoad(t *testing.T) {
	t.Run("over gRPC", func(t *testing.T) {
		ctx := gstgrpc.WithBearer(t.Context(), iamtest.GRPCSession(t, testsupport.Root))
		entries := pb.NewCacheEntryServiceClient(testutil.DialGRPC(t))

		stream, err := entries.LoadCacheEntry(ctx)
		require.NoError(t, err)
		for _, key := range []string{"loaded-a", "loaded-b", "loaded-c"} {
			require.NoError(t, stream.Send(&pb.LoadCacheEntryRequest{Payload: &pb.CacheEntryReq{Key: key, Value: "value of " + key}}))
		}
		rsp, err := stream.CloseAndRecv()
		require.NoError(t, err)
		require.EqualValues(t, 3, rsp.GetResult().GetCount())
		require.NotEmpty(t, rsp.GetResult().GetReplica(), "the reply names the replica that cached the entries")

		read, err := entries.GetCacheEntry(ctx, &pb.GetCacheEntryRequest{Id: "loaded-b"})
		require.NoError(t, err)
		require.True(t, read.GetResult().GetFound())
		require.Equal(t, "value of loaded-b", read.GetResult().GetValue())
	})

	t.Run("over WebSocket", func(t *testing.T) {
		cli := iamtest.Login(t, testsupport.Root).Client

		stream, err := cli.ClientStream[model.CacheEntryLoadReq, model.CacheEntryLoadRsp](t.Context(), "/api/cache_entries/load")
		require.NoError(t, err)
		defer stream.Close()
		for _, key := range []string{"loaded-d", "loaded-e"} {
			require.NoError(t, stream.Send(&model.CacheEntryLoadReq{Key: key, Value: "value of " + key}))
		}
		rsp, err := stream.CloseAndRecv()
		require.NoError(t, err)
		require.EqualValues(t, 2, rsp.Count)
		require.NotEmpty(t, rsp.Replica, "the reply names the replica that cached the entries")

		read, err := cli.Get[model.CacheEntryGetRsp](t.Context(), "/api/cache_entries/loaded-e")
		require.NoError(t, err)
		require.True(t, read.Found)
		require.Equal(t, "value of loaded-e", read.Value)
	})
}
