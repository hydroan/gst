package main_test

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/sse"
	"github.com/stretchr/testify/require"
)

const noticeStreamPath = "/api/notices"

// TestNoticeStream consumes the SSE route end to end: the request reaches the
// generated streaming route, the route delegates to the Streamer service, and
// the events arrive through the client's stream consumer.
func TestNoticeStream(t *testing.T) {
	cli, err := client.New(baseURL)
	require.NoError(t, err)

	var events []sse.Event
	err = cli.Stream(http.MethodGet, noticeStreamPath, nil, func(event sse.Event) error {
		events = append(events, event)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, events, 3)
	for i, event := range events {
		require.Equal(t, "notice", event.Event)
		require.Equal(t, strconv.Itoa(i+1), event.Data)
	}
}
