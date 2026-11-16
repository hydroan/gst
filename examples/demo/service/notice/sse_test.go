package notice_test

import (
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
)

// TestSSE covers GET /api/notices, served by Streamer in sse.go: the stream
// carries as many events as NOTICE_COUNT, two in TestMain, then ends.
func TestSSE(t *testing.T) {
	cli, err := client.New(testutil.BaseURL())
	require.NoError(t, err)

	events, err := cli.SSE(t.Context(), http.MethodGet, "/api/notices", nil)
	require.NoError(t, err)
	defer events.Close()

	var data []string
	for range 2 {
		event, recvErr := events.Recv()
		require.NoError(t, recvErr)
		require.Equal(t, "notice", event.Event)
		data = append(data, fmt.Sprint(event.Data))
	}
	require.Equal(t, []string{"1", "2"}, data)
	_, err = events.Recv()
	require.ErrorIs(t, err, io.EOF)
}
