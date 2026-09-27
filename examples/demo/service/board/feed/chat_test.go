package feed_test

import (
	"io"
	"testing"

	"demo/internal/testsupport"
	pbboard "demo/pb/board"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"
)

// TestChat covers the ChatFeed rpc, served by Chat in chat.go: a
// bidirectional stream echoing every event until the client closes its
// side.
func TestChat(t *testing.T) {
	feeds := pbboard.NewFeedServiceClient(testsupport.Dial(t))

	stream, err := feeds.ChatFeed(testsupport.Authorized(t.Context(), testsupport.Session(t)))
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
}
