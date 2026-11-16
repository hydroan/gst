package feed_test

import (
	"io"
	"testing"

	"demo/model/board"
	pbboard "demo/pb/board"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
)

// TestWatch covers the Stream action on /api/board/feeds/watch, served by
// Watch in watch.go: a server stream, public, answering three events of the
// topic and ending. Over gRPC it is the WatchFeed rpc, with the x-served-by
// header the project's interceptor adds to every call; over HTTP it is a
// WebSocket connection, opened through the framework client.
func TestWatch(t *testing.T) {
	want := []string{"news #1", "news #2", "news #3"}

	t.Run("over gRPC", func(t *testing.T) {
		feeds := pbboard.NewFeedServiceClient(testutil.DialGRPC(t))

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
		require.Equal(t, want, bodies)
		header, err := stream.Header()
		require.NoError(t, err)
		require.Equal(t, []string{"demo"}, header.Get("x-served-by"))
	})

	t.Run("over WebSocket", func(t *testing.T) {
		cli, err := client.New(testutil.BaseURL())
		require.NoError(t, err)

		stream, err := cli.ServerStream[board.FeedWatchRsp](t.Context(), "/api/board/feeds/watch", &board.FeedWatchReq{Topic: "news"})
		require.NoError(t, err)
		defer stream.Close()
		var bodies []string
		for {
			rsp, recvErr := stream.Recv()
			if errors.Is(recvErr, io.EOF) {
				break
			}
			require.NoError(t, recvErr)
			bodies = append(bodies, rsp.Body)
		}
		require.Equal(t, want, bodies)
	})
}
