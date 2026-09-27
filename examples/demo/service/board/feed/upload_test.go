package feed_test

import (
	"testing"

	"demo/internal/testsupport"
	pbboard "demo/pb/board"

	"github.com/hydroan/gst/client"
	"github.com/stretchr/testify/require"
)

// TestUpload covers the UploadFeed rpc, served by Upload in upload.go: a
// client stream, counting the events until the client closes its side.
// Behind the session check the project's actor interceptor names the caller
// in the x-actor header.
func TestUpload(t *testing.T) {
	feeds := pbboard.NewFeedServiceClient(testsupport.Dial(t))
	account := testsupport.Login(t, client.WithUserAgent(testsupport.GRPCUserAgent))

	stream, err := feeds.UploadFeed(testsupport.Authorized(t.Context(), account.SessionID))
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
}
