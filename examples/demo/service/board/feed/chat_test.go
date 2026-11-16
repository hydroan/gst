package feed_test

import (
	"io"
	"testing"

	"demo/internal/testsupport"
	"demo/model/board"
	pbboard "demo/pb/board"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/client"
	gstgrpc "github.com/hydroan/gst/grpc"
	"github.com/hydroan/gst/testutil"
	"github.com/hydroan/gst/testutil/iamtest"
	"github.com/stretchr/testify/require"
)

// TestChat covers the Stream action on /api/board/feeds/chat, served by Chat
// in chat.go: a bidirectional stream behind the session check, echoing every
// event until the client closes its side. Over gRPC it is the ChatFeed rpc;
// over HTTP it is a WebSocket connection.
func TestChat(t *testing.T) {
	t.Run("over gRPC", func(t *testing.T) {
		feeds := pbboard.NewFeedServiceClient(testutil.DialGRPC(t))

		stream, err := feeds.ChatFeed(gstgrpc.WithBearer(t.Context(), testsupport.Login(t, client.WithUserAgent(iamtest.GRPCUserAgent)).SessionID))
		require.NoError(t, err)
		for seq, body := range []string{"hello", "again"} {
			require.NoError(t, stream.Send(&pbboard.ChatFeedRequest{Payload: &pbboard.FeedEvent{Seq: int64(seq + 1), Body: body}}))
			rsp, recvErr := stream.Recv()
			require.NoError(t, recvErr)
			require.Equal(t, int64(seq+1), rsp.GetResult().GetSeq())
			require.Equal(t, "echo: "+body, rsp.GetResult().GetBody())
		}
		require.NoError(t, stream.CloseSend())
		_, err = stream.Recv()
		require.True(t, errors.Is(err, io.EOF), "the server ends the stream once the client has: %v", err)
	})

	t.Run("over WebSocket", func(t *testing.T) {
		stream, err := testsupport.Login(t).Client.BidiStream[board.FeedChatReq, board.FeedChatRsp](t.Context(), "/api/board/feeds/chat")
		require.NoError(t, err)
		defer stream.Close()
		for seq, body := range []string{"hello", "again"} {
			require.NoError(t, stream.Send(&board.FeedChatReq{Seq: int64(seq + 1), Body: body}))
			rsp, recvErr := stream.Recv()
			require.NoError(t, recvErr)
			require.Equal(t, int64(seq+1), rsp.Seq)
			require.Equal(t, "echo: "+body, rsp.Body)
		}
		require.NoError(t, stream.CloseSend())
		_, err = stream.Recv()
		require.ErrorIs(t, err, io.EOF, "the server ends the stream once the client has")
	})
}
