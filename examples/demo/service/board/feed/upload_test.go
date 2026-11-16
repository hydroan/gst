package feed_test

import (
	"testing"

	"demo/internal/testsupport"
	"demo/model/board"
	pbboard "demo/pb/board"

	"github.com/hydroan/gst/client"
	gstgrpc "github.com/hydroan/gst/grpc"
	"github.com/hydroan/gst/testutil"
	"github.com/hydroan/gst/testutil/iamtest"
	"github.com/stretchr/testify/require"
)

// TestUpload covers the Stream action on /api/board/feeds/upload, served by
// Upload in upload.go: a client stream behind the session check, counting
// the events until the client closes its side. Over gRPC it is the
// UploadFeed rpc, the session traveling as a bearer token and the project's
// actor interceptor naming the caller in the x-actor header; over HTTP it is
// a WebSocket connection, the session traveling in the cookie the login
// set.
func TestUpload(t *testing.T) {
	t.Run("over gRPC", func(t *testing.T) {
		feeds := pbboard.NewFeedServiceClient(testutil.DialGRPC(t))
		account := testsupport.Login(t, client.WithUserAgent(iamtest.GRPCUserAgent))

		stream, err := feeds.UploadFeed(gstgrpc.WithBearer(t.Context(), account.SessionID))
		require.NoError(t, err)
		for seq := int64(1); seq <= 4; seq++ {
			require.NoError(t, stream.Send(&pbboard.UploadFeedRequest{Payload: &pbboard.FeedEvent{Seq: seq, Body: "event"}}))
		}
		rsp, err := stream.CloseAndRecv()
		require.NoError(t, err)
		require.Equal(t, int64(4), rsp.GetResult().GetAccepted())
		header, err := stream.Header()
		require.NoError(t, err)
		require.Equal(t, []string{account.Username}, header.Get("x-actor"))
	})

	t.Run("over WebSocket", func(t *testing.T) {
		account := testsupport.Login(t)

		stream, err := account.Client.ClientStream[board.FeedUploadReq, board.FeedUploadRsp](t.Context(), "/api/board/feeds/upload")
		require.NoError(t, err)
		defer stream.Close()
		for seq := int64(1); seq <= 4; seq++ {
			require.NoError(t, stream.Send(&board.FeedUploadReq{Seq: seq, Body: "event"}))
		}
		rsp, err := stream.CloseAndRecv()
		require.NoError(t, err)
		require.Equal(t, int64(4), rsp.Accepted)
	})
}
