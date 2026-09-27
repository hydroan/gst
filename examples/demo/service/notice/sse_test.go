package notice_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/sse"
	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
)

// TestSSE covers GET /api/notices, served by Streamer in sse.go: the stream
// carries as many events as NOTICE_COUNT, two in TestMain, then ends.
func TestSSE(t *testing.T) {
	cli, err := client.New(testutil.BaseURL())
	require.NoError(t, err)

	var data []string
	err = cli.Stream(t.Context(), http.MethodGet, "/api/notices", nil, func(event sse.Event) error {
		require.Equal(t, "notice", event.Event)
		data = append(data, fmt.Sprint(event.Data))
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, []string{"1", "2"}, data)
}
