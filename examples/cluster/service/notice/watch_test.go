package notice_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"cluster/helper"
	"cluster/internal/testsupport"
	"cluster/model"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/testutil"
	"github.com/hydroan/gst/testutil/iamtest"
	"github.com/stretchr/testify/require"
)

// TestWatch covers GET /api/notices/watch, served by Watch in watch.go: a
// WebSocket connection behind the session check that carries every notice
// published while it is open. One process stands for the replica that
// publishes and for the one each connection is on; a notice crossing from one
// replica to the connections of another is what the deployment scenarios in
// the README check.
func TestWatch(t *testing.T) {
	t.Run("every watching connection receives the notice", func(t *testing.T) {
		cli := iamtest.Login(t, testsupport.Root).Client
		// A watch ends only when its client leaves: the deadline is what
		// ends the wait for a notice that never arrives.
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
		defer cancel()

		first, err := cli.ServerStream[model.NoticeWatchRsp](ctx, "/api/notices/watch", nil)
		require.NoError(t, err)
		defer first.Close()
		second, err := cli.ServerStream[model.NoticeWatchRsp](ctx, "/api/notices/watch", nil)
		require.NoError(t, err)
		defer second.Close()

		// A connection is open before its subscription is, and one a test
		// before this one closed may not have ended its own yet: the notice
		// published while exactly these two are subscribed is the one both
		// carry, and no earlier one reached either.
		require.Eventually(t, func() bool {
			rsp, postErr := cli.Post[model.NoticeCreateRsp](t.Context(), "/api/notices", model.NoticeReq{Text: "hello"})
			return postErr == nil && rsp.Watchers == 2
		}, 10*time.Second, 50*time.Millisecond, "the two connections never were the only ones watching")

		for _, stream := range []*client.ServerStream[model.NoticeWatchRsp]{first, second} {
			notice, recvErr := stream.Recv()
			require.NoError(t, recvErr)
			require.Equal(t, "hello", notice.Text)
			require.Equal(t, helper.Replica(), notice.From, "the notice names the replica that published it")
			require.Equal(t, helper.Replica(), notice.Replica, "and the replica the connection is on")
		}
	})

	t.Run("without a session", func(t *testing.T) {
		cli, err := client.New(testutil.BaseURL())
		require.NoError(t, err)

		_, err = cli.ServerStream[model.NoticeWatchRsp](t.Context(), "/api/notices/watch", nil)
		testutil.RequireError(t, err, http.StatusUnauthorized)
	})
}
