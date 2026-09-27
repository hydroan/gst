package feed_test

import (
	"io"
	"testing"

	"demo/internal/testsupport"
	pbboard "demo/pb/board"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"
)

// TestWatch covers the WatchFeed rpc, served by Watch in watch.go: a server
// stream, public, answering three events of the topic and ending, with the
// x-served-by header the project's interceptor adds to every call.
func TestWatch(t *testing.T) {
	feeds := pbboard.NewFeedServiceClient(testsupport.Dial(t))

	stream, err := feeds.WatchFeed(t.Context(), &pbboard.WatchFeedRequest{Payload: &pbboard.FeedWatchReq{Topic: "news"}})
	require.NoError(t, err)
	var bodies []string
	for {
		rsp, recvErr := stream.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		require.NoError(t, recvErr)
		bodies = append(bodies, rsp.GetResult().GetBody())
	}
	require.Equal(t, []string{"news #1", "news #2", "news #3"}, bodies)
	header, err := stream.Header()
	require.NoError(t, err)
	require.Equal(t, []string{"demo"}, header.Get("x-served-by"))
}
