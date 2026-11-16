package router_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/middleware"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/router"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
)

// The three routes below wire the Stream phase the way generated code does
// (see sse_test.go), one stream of each kind. The engine TestMain brings up
// serves them behind the framework's whole middleware chain, which is what
// these tests are about: the handshake of a stream passes the chain like any
// request, and the connection it opens outlives it.
const (
	streamWatchRoute  = "feeds/:feed/watch"
	streamUploadRoute = "feeds/upload"
	streamChatRoute   = "feeds/chat"
)

// admittedOrigin is a page of the origins TestMain configures the server to
// admit beside its own host, and refusedNote the note feedWatcher refuses.
const (
	admittedOrigin = "https://app.sample.example"
	refusedNote    = "refused"
)

// feedNote is the message the streams of these tests carry, a request or a
// response.
type feedNote struct {
	Note string `json:"note"`
}

// feedWatcher serves the server stream: it answers with the feed its route
// names, then with the note of its request, and refuses the note refusedNote
// the way a service refuses a request.
type feedWatcher struct {
	serviceregistry.Base[*modelregistry.Empty, *feedNote, *feedNote]
}

func (s *feedWatcher) Stream(ctx *types.ServiceContext, req *feedNote, stream *types.ServerStream[*feedNote]) error {
	if req.Note == refusedNote {
		return types.NewError(http.StatusConflict, "the feed takes no such note")
	}
	if err := stream.Send(&feedNote{Note: ctx.Param("feed")}); err != nil {
		return err
	}
	return stream.Send(req)
}

// feedUploader serves the client stream: it answers the notes it was sent,
// joined.
type feedUploader struct {
	serviceregistry.Base[*modelregistry.Empty, *feedNote, *feedNote]
}

func (s *feedUploader) Stream(_ *types.ServiceContext, stream *types.ClientStream[*feedNote]) (*feedNote, error) {
	var notes []string
	for {
		req, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return &feedNote{Note: strings.Join(notes, ",")}, nil
		}
		if err != nil {
			return nil, err
		}
		notes = append(notes, req.Note)
	}
}

// feedChatter serves the bidirectional stream: it answers every note as it
// arrives.
type feedChatter struct {
	serviceregistry.Base[*modelregistry.Empty, *feedNote, *feedNote]
}

func (s *feedChatter) Stream(_ *types.ServiceContext, stream *types.BidiStream[*feedNote, *feedNote]) error {
	for {
		req, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err = stream.Send(req); err != nil {
			return err
		}
	}
}

// The services register in init and the routes after router.Init, the split
// generated code has (see sse_test.go).
func init() {
	serviceregistry.RegisterInstance[*modelregistry.Empty, *feedNote, *feedNote](apiaction.Stream, streamWatchRoute, &feedWatcher{})
	serviceregistry.RegisterInstance[*modelregistry.Empty, *feedNote, *feedNote](apiaction.Stream, streamUploadRoute, &feedUploader{})
	serviceregistry.RegisterInstance[*modelregistry.Empty, *feedNote, *feedNote](apiaction.Stream, streamChatRoute, &feedChatter{})
}

// registerStreamRoutes registers the stream routes; TestMain calls it after
// router.Init, mirroring where generated route registration runs.
func registerStreamRoutes() {
	for _, route := range []string{streamWatchRoute, streamUploadRoute, streamChatRoute} {
		router.Register[*modelregistry.Empty, *feedNote, *feedNote](
			router.Auth(), route, &types.ControllerConfig[*modelregistry.Empty]{}, apiaction.Stream,
		)
	}
}

// TestStreamRouteServesAServerStream proves a Stream route serves a server
// stream over a WebSocket connection: the handshake carries the route
// parameter, the request is the client's one message, and the responses
// arrive until the service returns. A service error ends the stream as the
// rejection it is.
func TestStreamRouteServesAServerStream(t *testing.T) {
	cli, err := client.New(baseURL)
	require.NoError(t, err)

	stream, err := cli.ServerStream[feedNote](streamContext(t), "/api/feeds/f-1/watch", &feedNote{Note: "first"})
	require.NoError(t, err)
	defer stream.Close()
	for _, want := range []string{"f-1", "first"} {
		rsp, recvErr := stream.Recv()
		require.NoError(t, recvErr)
		require.Equal(t, want, rsp.Note)
	}
	_, err = stream.Recv()
	require.ErrorIs(t, err, io.EOF, "the stream ends once the service returns")

	t.Run("a service error", func(t *testing.T) {
		refused, err := cli.ServerStream[feedNote](streamContext(t), "/api/feeds/f-1/watch", &feedNote{Note: refusedNote})
		require.NoError(t, err)
		defer refused.Close()
		_, err = refused.Recv()
		var rejection *client.Error
		require.ErrorAs(t, err, &rejection)
		require.Equal(t, http.StatusConflict, rejection.StatusCode)
		require.Equal(t, "the feed takes no such note", rejection.Msg)
	})
}

// TestStreamRouteServesAClientStream proves a Stream route serves a client
// stream: the requests arrive one by one, and the response answers them once
// the client is through.
func TestStreamRouteServesAClientStream(t *testing.T) {
	cli, err := client.New(baseURL)
	require.NoError(t, err)

	stream, err := cli.ClientStream[feedNote, feedNote](streamContext(t), "/api/"+streamUploadRoute)
	require.NoError(t, err)
	defer stream.Close()
	for _, note := range []string{"a", "b"} {
		require.NoError(t, stream.Send(&feedNote{Note: note}))
	}
	rsp, err := stream.CloseAndRecv()
	require.NoError(t, err)
	require.Equal(t, "a,b", rsp.Note)
}

// TestStreamRouteServesABidirectionalStream proves a Stream route serves a
// bidirectional stream: every request is answered as it is sent, until the
// client is through.
func TestStreamRouteServesABidirectionalStream(t *testing.T) {
	cli, err := client.New(baseURL)
	require.NoError(t, err)

	stream, err := cli.BidiStream[feedNote, feedNote](streamContext(t), "/api/"+streamChatRoute)
	require.NoError(t, err)
	defer stream.Close()
	for _, note := range []string{"a", "b"} {
		require.NoError(t, stream.Send(&feedNote{Note: note}))
		rsp, recvErr := stream.Recv()
		require.NoError(t, recvErr)
		require.Equal(t, note, rsp.Note)
	}
	require.NoError(t, stream.CloseSend())
	_, err = stream.Recv()
	require.ErrorIs(t, err, io.EOF)
}

// TestStreamRouteAdmitsThePagesOfTheOriginsConfigured proves which pages may
// open the stream of a route: the ones of an origin server.websocket_origins
// lists, and no other, whose handshake is answered as a rejection like any
// request the server refuses.
func TestStreamRouteAdmitsThePagesOfTheOriginsConfigured(t *testing.T) {
	// from opens the chat stream the way a page of origin does.
	from := func(t *testing.T, origin string) (*client.BidiStream[feedNote, feedNote], error) {
		t.Helper()

		cli, err := client.New(baseURL, client.WithHeader(http.Header{"Origin": {origin}}))
		require.NoError(t, err)
		return cli.BidiStream[feedNote, feedNote](streamContext(t), "/api/"+streamChatRoute)
	}

	stream, err := from(t, admittedOrigin)
	require.NoError(t, err)
	defer stream.Close()
	require.NoError(t, stream.Send(&feedNote{Note: "a"}))
	rsp, err := stream.Recv()
	require.NoError(t, err)
	require.Equal(t, "a", rsp.Note)

	stream, err = from(t, "https://app.other.example")
	require.Nil(t, stream)
	var rejection *client.Error
	require.ErrorAs(t, err, &rejection)
	require.Equal(t, http.StatusForbidden, rejection.StatusCode)
}

// TestStreamRouteIsMarkedStreaming pins that the Stream phase marks its
// route as streaming, the mark the request-scoped middlewares leave a
// long-lived connection alone by.
func TestStreamRouteIsMarkedStreaming(t *testing.T) {
	require.True(t, middleware.IsStreamingRoute(http.MethodGet, "/api/"+streamWatchRoute))
	require.False(t, middleware.IsStreamingRoute(http.MethodPost, "/api/"+streamWatchRoute))
}

// TestStreamRouteIsLeftOutOfTheOpenAPIDocument pins that the document, which
// describes a request and its response, holds no entry for a route answering
// by opening a connection, beside the entries of the other routes.
func TestStreamRouteIsLeftOutOfTheOpenAPIDocument(t *testing.T) {
	document := requireOpenAPIDocument(t)

	require.Contains(t, document.Paths, "/api/"+pagedRecordRoute)
	for _, path := range []string{"/api/feeds/{feed}/watch", "/api/" + streamUploadRoute, "/api/" + streamChatRoute} {
		require.NotContains(t, document.Paths, path)
	}
}

// streamContext returns the context a test opens a stream on: bound, so
// that a stream nothing ends fails its test instead of holding it.
func streamContext(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}
