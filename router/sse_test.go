package router_test

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/middleware"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/router"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/sse"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
)

// The two routes below wire the SSE verb the way generated code does: the
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

func (s *noticeStreamer) SSE(ctx *types.ServiceContext) error {
	return ctx.SSE(func(conn *sse.Conn) error {
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

func (s *endlessStreamer) SSE(ctx *types.ServiceContext) error {
	return ctx.SSE(func(conn *sse.Conn) error {
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
	service.Register[*noticeStreamer](consts.PHASE_SSE, sseStreamRoute)
	service.Register[*endlessStreamer](consts.PHASE_SSE, sseEndlessRoute)
}

// registerSSERoutes registers the streaming routes; TestMain calls it after
// router.Init, mirroring where generated route registration runs.
func registerSSERoutes() {
	router.Register[*model.Empty, *model.Empty, *model.Empty](
		router.Auth(), sseStreamRoute, &types.ControllerConfig[*model.Empty]{}, consts.SSE,
	)
	router.Register[*model.Empty, *model.Empty, *model.Empty](
		router.Auth(), sseEndlessRoute, &types.ControllerConfig[*model.Empty]{}, consts.SSE,
	)
}

func TestSSERouteStreamsEvents(t *testing.T) {
	cli, err := client.New(baseURL)
	require.NoError(t, err)

	var events []sse.Event
	err = cli.Stream(http.MethodGet, "/api/"+sseStreamRoute, nil, func(event sse.Event) error {
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

func TestSSERouteIsMarkedStreaming(t *testing.T) {
	require.True(t, middleware.IsStreamingRoute(http.MethodGet, "/api/"+sseStreamRoute),
		"the SSE verb must mark its route as streaming for the middleware exemptions")
	require.False(t, middleware.IsStreamingRoute(http.MethodPost, "/api/"+sseStreamRoute))
}

func TestSSEStreamStopsOnClientRequest(t *testing.T) {
	cli, err := client.New(baseURL)
	require.NoError(t, err)

	var seen int
	err = cli.Stream(http.MethodGet, "/api/"+sseEndlessRoute, nil, func(sse.Event) error {
		seen++
		if seen == 2 {
			return client.ErrStopStream
		}
		return nil
	})
	require.NoError(t, err, "ErrStopStream ends consumption without surfacing an error")
	require.Equal(t, 2, seen)
}
