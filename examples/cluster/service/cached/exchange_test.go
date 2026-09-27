package cached_test

import (
	"io"
	"testing"

	"cluster/internal/testsupport"
	"cluster/pb"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"
)

// TestExchange covers the ExchangeCached rpc, served by Exchange in
// exchange.go: a stream both ways, each key sent answered with what this
// replica holds, a key it never received answered as missing, and the
// stream ending once the client closes its side.
func TestExchange(t *testing.T) {
	ctx := testsupport.Authorized(t.Context(), testsupport.Session(t))
	caches := pb.NewCachedServiceClient(testsupport.Dial(t))
	_, err := caches.CreateCached(ctx, &pb.CreateCachedRequest{Payload: &pb.CachedReq{Key: "exchanged-key", Value: "exchanged-value"}})
	require.NoError(t, err)

	stream, err := caches.ExchangeCached(ctx)
	require.NoError(t, err)

	require.NoError(t, stream.Send(&pb.ExchangeCachedRequest{Payload: &pb.CachedKeyReq{Key: "exchanged-key"}}))
	rsp, err := stream.Recv()
	require.NoError(t, err)
	require.True(t, rsp.GetResult().GetFound())
	require.Equal(t, "exchanged-value", rsp.GetResult().GetValue())
	require.NotEmpty(t, rsp.GetResult().GetReplica())

	require.NoError(t, stream.Send(&pb.ExchangeCachedRequest{Payload: &pb.CachedKeyReq{Key: "never-exchanged-key"}}))
	rsp, err = stream.Recv()
	require.NoError(t, err)
	require.False(t, rsp.GetResult().GetFound(), "a key this replica never received reads as missing")

	require.NoError(t, stream.CloseSend())
	_, err = stream.Recv()
	require.True(t, errors.Is(err, io.EOF), "the stream ends once the client has closed its side, got %v", err)
}
