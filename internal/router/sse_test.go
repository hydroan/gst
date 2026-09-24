package router_test

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/middleware"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/router"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/internal/sse"
	"github.com/hydroan/gst/internal/types"
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
	serviceregistry.Base[*modelregistry.Empty, *modelregistry.Empty, *modelregistry.Empty]
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
	serviceregistry.Base[*modelregistry.Empty, *modelregistry.Empty, *modelregistry.Empty]
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
	serviceregistry.Register[*modelregistry.Empty, *modelregistry.Empty, *modelregistry.Empty](consts.PHASE_SSE, sseStreamRoute, &noticeStreamer{})
	serviceregistry.Register[*modelregistry.Empty, *modelregistry.Empty, *modelregistry.Empty](consts.PHASE_SSE, sseEndlessRoute, &endlessStreamer{})
}

// registerSSERoutes registers the streaming routes; TestMain calls it after
// router.Init, mirroring where generated route registration runs.
func registerSSERoutes() {
	router.Register[*modelregistry.Empty, *modelregistry.Empty, *modelregistry.Empty](
		router.Auth(), sseStreamRoute, &types.ControllerConfig[*modelregistry.Empty]{}, consts.SSE,
	)
	router.Register[*modelregistry.Empty, *modelregistry.Empty, *modelregistry.Empty](
		router.Auth(), sseEndlessRoute, &types.ControllerConfig[*modelregistry.Empty]{}, consts.SSE,
	)
}

func TestSSERouteStreamsEvents(t *testing.T) {
	cli, err := client.New(baseURL)
	require.NoError(t, err)

	var events []sse.Event
	err = cli.Stream(t.Context(), http.MethodGet, "/api/"+sseStreamRoute, nil, func(event sse.Event) error {
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
	err = cli.Stream(t.Context(), http.MethodGet, "/api/"+sseEndlessRoute, nil, func(sse.Event) error {
		seen++
		if seen == 2 {
			return client.ErrStopStream
		}
		return nil
	})
	require.NoError(t, err, "ErrStopStream ends consumption without surfacing an error")
	require.Equal(t, 2, seen)
}
