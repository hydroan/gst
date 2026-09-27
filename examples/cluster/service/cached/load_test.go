package cached_test

import (
	"testing"

	"cluster/internal/testsupport"
	"cluster/pb"

	"github.com/stretchr/testify/require"
)

// TestLoad covers the LoadCached rpc, served by Load in load.go: a client
// stream of entries is cached on the replica that took it, which answers
// the count once the client has closed its side, and the entries read back
// from that replica's store.
func TestLoad(t *testing.T) {
	ctx := testsupport.Authorized(t.Context(), testsupport.Session(t))
	caches := pb.NewCachedServiceClient(testsupport.Dial(t))

	stream, err := caches.LoadCached(ctx)
	require.NoError(t, err)
	for _, key := range []string{"loaded-a", "loaded-b", "loaded-c"} {
		require.NoError(t, stream.Send(&pb.LoadCachedRequest{Payload: &pb.CachedReq{Key: key, Value: "value of " + key}}))
	}
	rsp, err := stream.CloseAndRecv()
	require.NoError(t, err)
	require.EqualValues(t, 3, rsp.GetResult().GetCount())
	require.NotEmpty(t, rsp.GetResult().GetReplica(), "the reply names the replica that cached the entries")

	read, err := caches.GetCached(ctx, &pb.GetCachedRequest{Id: "loaded-b"})
	require.NoError(t, err)
	require.True(t, read.GetResult().GetFound())
	require.Equal(t, "value of loaded-b", read.GetResult().GetValue())
}
