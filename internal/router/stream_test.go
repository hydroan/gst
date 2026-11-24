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
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/middleware"
	"github.com/hydroan/gst/internal/model"
	"github.com/hydroan/gst/internal/router"
	"github.com/hydroan/gst/internal/service"
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
	service.Base[*model.Empty, *feedNote, *feedNote]
}

func (s *feedWatcher) Stream(ctx *gst.ServiceContext, req *feedNote, stream *gst.ServerStream[*feedNote]) error {
	if req.Note == refusedNote {
		return gst.NewError(http.StatusConflict, "the feed takes no such note")
	}
	if err := stream.Send(&feedNote{Note: ctx.Param("feed")}); err != nil {
		return err
	}
	return stream.Send(req)
}

// feedUploader serves the client stream: it answers the notes it was sent,
// joined.
type feedUploader struct {
	service.Base[*model.Empty, *feedNote, *feedNote]
}

func (s *feedUploader) Stream(_ *gst.ServiceContext, stream *gst.ClientStream[*feedNote]) (*feedNote, error) {
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
	service.Base[*model.Empty, *feedNote, *feedNote]
}

func (s *feedChatter) Stream(_ *gst.ServiceContext, stream *gst.BidiStream[*feedNote, *feedNote]) error {
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
	service.RegisterInstance[*model.Empty, *feedNote, *feedNote](apiaction.Stream, streamWatchRoute, &feedWatcher{})
	service.RegisterInstance[*model.Empty, *feedNote, *feedNote](apiaction.Stream, streamUploadRoute, &feedUploader{})
	service.RegisterInstance[*model.Empty, *feedNote, *feedNote](apiaction.Stream, streamChatRoute, &feedChatter{})
}

// registerStreamRoutes registers the stream routes; TestMain calls it after
// router.Init, mirroring where generated route registration runs.
func registerStreamRoutes() {
	for _, route := range []string{streamWatchRoute, streamUploadRoute, streamChatRoute} {
		router.Register[*model.Empty, *feedNote, *feedNote](
			router.Auth(), route, &gst.ControllerConfig[*model.Empty]{}, apiaction.Stream,
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

// TestStreamRouteIsDocumentedAsTheRequestOpeningItsConnection pins that the
// OpenAPI document the server serves holds the route of every Stream action
// as the GET that opens its connection, with the parameter its path carries.
func TestStreamRouteIsDocumentedAsTheRequestOpeningItsConnection(t *testing.T) {
	document := requireOpenAPIDocument(t)

	for _, path := range []string{"/api/feeds/{feed}/watch", "/api/" + streamUploadRoute, "/api/" + streamChatRoute} {
		require.Contains(t, document.Paths[path], "get", "GET %s", path)
	}
	parameters := document.Paths["/api/feeds/{feed}/watch"]["get"].Parameters
	require.Len(t, parameters, 1)
	require.Equal(t, "feed", parameters[0].Name)
	require.Equal(t, "path", parameters[0].In)
}

// streamContext returns the context a test opens a stream on: bound, so
// that a stream nothing ends fails its test instead of holding it.
func streamContext(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}
