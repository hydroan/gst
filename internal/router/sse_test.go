package router_test

import (
	"io"
	"net/http"
	"strconv"
	"testing"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/middleware"
	"github.com/hydroan/gst/internal/model"
	"github.com/hydroan/gst/internal/router"
	"github.com/hydroan/gst/internal/service"
	"github.com/hydroan/gst/internal/sse"
	"github.com/stretchr/testify/require"
)

// The two routes below wire the SSE phase the way generated code does: the
// service registers under the same raw route the router registers, and the
// controller resolves it through the route-derived registry key.
const (
	sseStreamRoute  = "notices"
	sseEndlessRoute = "notices/endless"
)

// noticeStreamer streams a fixed number of events and ends the stream.
type noticeStreamer struct {
	service.Base[*model.Empty, *model.Empty, *model.Empty]
}

func (s *noticeStreamer) SSE(ctx *gst.ServiceContext) error {
	return sse.Serve(ctx, func(conn *sse.Conn) error {
		for i := 1; i <= 3; i++ {
			if err := conn.Send(sse.Event{Event: "notice", Data: i}); err != nil {
				return err
			}
		}
		return nil
	})
}

// endlessStreamer streams until the client goes away, the shape of a real
// event feed.
type endlessStreamer struct {
	service.Base[*model.Empty, *model.Empty, *model.Empty]
}

func (s *endlessStreamer) SSE(ctx *gst.ServiceContext) error {
	return sse.Serve(ctx, func(conn *sse.Conn) error {
		for i := 1; ; i++ {
			select {
			case <-conn.Context().Done():
				return nil
			default:
			}
			if err := conn.Send(sse.Event{Event: "notice", Data: i}); err != nil {
				return err
			}
		}
	})
}

// The services register in init because service registration has to happen
// before the framework bootstraps, while the routes below register after
// router.Init — the same split generated code has.
func init() {
	service.RegisterInstance[*model.Empty, *model.Empty, *model.Empty](apiaction.SSE, sseStreamRoute, &noticeStreamer{})
	service.RegisterInstance[*model.Empty, *model.Empty, *model.Empty](apiaction.SSE, sseEndlessRoute, &endlessStreamer{})
}

// registerSSERoutes registers the streaming routes; TestMain calls it after
// router.Init, mirroring where generated route registration runs.
func registerSSERoutes() {
	router.Register[*model.Empty, *model.Empty, *model.Empty](
		router.Auth(), sseStreamRoute, &gst.ControllerConfig[*model.Empty]{}, apiaction.SSE,
	)
	router.Register[*model.Empty, *model.Empty, *model.Empty](
		router.Auth(), sseEndlessRoute, &gst.ControllerConfig[*model.Empty]{}, apiaction.SSE,
	)
}

func TestSSERouteStreamsEvents(t *testing.T) {
	cli, err := client.New(baseURL)
	require.NoError(t, err)

	events, err := cli.SSE(t.Context(), http.MethodGet, "/api/"+sseStreamRoute, nil)
	require.NoError(t, err)
	defer events.Close()
	for i := range 3 {
		event, recvErr := events.Recv()
		require.NoError(t, recvErr)
		require.Equal(t, "notice", event.Event)
		require.Equal(t, strconv.Itoa(i+1), event.Data)
	}
	_, err = events.Recv()
	require.ErrorIs(t, err, io.EOF, "the stream ends once the service returns")
}

func TestSSERouteIsMarkedStreaming(t *testing.T) {
	require.True(t, middleware.IsStreamingRoute(http.MethodGet, "/api/"+sseStreamRoute),
		"the SSE phase must mark its route as streaming for the middleware exemptions")
	require.False(t, middleware.IsStreamingRoute(http.MethodPost, "/api/"+sseStreamRoute))
}

func TestSSEStreamStopsOnClientRequest(t *testing.T) {
	cli, err := client.New(baseURL)
	require.NoError(t, err)

	events, err := cli.SSE(t.Context(), http.MethodGet, "/api/"+sseEndlessRoute, nil)
	require.NoError(t, err)
	for range 2 {
		_, err = events.Recv()
		require.NoError(t, err)
	}
	require.NoError(t, events.Close(), "closing the stream ends consumption without surfacing an error")
}
