package cacheentry_test

import (
	"io"
	"testing"

	"cluster/internal/testsupport"
	"cluster/model"
	"cluster/pb"

	"github.com/cockroachdb/errors"
	gstgrpc "github.com/hydroan/gst/grpc"
	"github.com/hydroan/gst/testutil"
	"github.com/hydroan/gst/testutil/iamtest"
	"github.com/stretchr/testify/require"
)

// TestExchange covers the Stream action on /api/cache_entries/exchange,
// served by Exchange in exchange.go: a stream both ways, each key sent
// answered with what this replica holds, a key it never received answered
// as missing, and the stream ending once the client closes its side. Over
// gRPC it is the ExchangeCacheEntry rpc, the session traveling as a bearer
// token; over HTTP it is a WebSocket connection, the session traveling in
// the cookie the login set.
func TestExchange(t *testing.T) {
	t.Run("over gRPC", func(t *testing.T) {
		ctx := gstgrpc.WithBearer(t.Context(), iamtest.GRPCSession(t, testsupport.Root))
		entries := pb.NewCacheEntryServiceClient(testutil.DialGRPC(t))
		_, err := entries.CreateCacheEntry(ctx, &pb.CreateCacheEntryRequest{Payload: &pb.CacheEntryReq{Key: "exchanged-key", Value: "exchanged-value"}})
		require.NoError(t, err)

		stream, err := entries.ExchangeCacheEntry(ctx)
		require.NoError(t, err)

		require.NoError(t, stream.Send(&pb.ExchangeCacheEntryRequest{Payload: &pb.CacheEntryKeyReq{Key: "exchanged-key"}}))
		rsp, err := stream.Recv()
		require.NoError(t, err)
		require.True(t, rsp.GetResult().GetFound())
		require.Equal(t, "exchanged-value", rsp.GetResult().GetValue())
		require.NotEmpty(t, rsp.GetResult().GetReplica())

		require.NoError(t, stream.Send(&pb.ExchangeCacheEntryRequest{Payload: &pb.CacheEntryKeyReq{Key: "never-exchanged-key"}}))
		rsp, err = stream.Recv()
		require.NoError(t, err)
		require.False(t, rsp.GetResult().GetFound(), "a key this replica never received reads as missing")

		require.NoError(t, stream.CloseSend())
		_, err = stream.Recv()
		require.True(t, errors.Is(err, io.EOF), "the stream ends once the client has closed its side, got %v", err)
	})

	t.Run("over WebSocket", func(t *testing.T) {
		cli := iamtest.Login(t, testsupport.Root).Client
		_, err := cli.Post[model.CacheEntryCreateRsp](t.Context(), "/api/cache_entries", model.CacheEntryReq{Key: "exchanged-over-websocket", Value: "exchanged-value"})
		require.NoError(t, err)

		stream, err := cli.BidiStream[model.CacheEntryKeyReq, model.CacheEntryExchangeRsp](t.Context(), "/api/cache_entries/exchange")
		require.NoError(t, err)
		defer stream.Close()

		require.NoError(t, stream.Send(&model.CacheEntryKeyReq{Key: "exchanged-over-websocket"}))
		rsp, err := stream.Recv()
		require.NoError(t, err)
		require.True(t, rsp.Found)
		require.Equal(t, "exchanged-value", rsp.Value)
		require.NotEmpty(t, rsp.Replica)

		require.NoError(t, stream.Send(&model.CacheEntryKeyReq{Key: "never-exchanged-key"}))
		rsp, err = stream.Recv()
		require.NoError(t, err)
		require.False(t, rsp.Found, "a key this replica never received reads as missing")

		require.NoError(t, stream.CloseSend())
		_, err = stream.Recv()
		require.ErrorIs(t, err, io.EOF, "the connection closes once the client has closed its side")
	})
}
