package controller_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/controller"
	"github.com/hydroan/gst/internal/grpcserver"
	"github.com/hydroan/gst/internal/middleware"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/streamctx"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/internal/websocket"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestStreamHandlerStreamsTheResponsesOfARequest pins the server stream over
// HTTP end to end: the service's Stream method gets the request the first
// message carried and a service context naming the caller, the route and its
// parameters, sends its responses one message each, and the connection closes
// normally when it returns, the status left for the access log. A request the
// validator or the decoding refuses closes the stream as the 400 it is, the
// service's error with the status it stands for, any other error as the
// server's failure, a message after the request as a violation, and a request
// that does not arrive in time as a 408; a client ending its messages after
// the request is taken at its word, one ending them without a request sends
// the zero request, a stream ends with its client leaving or the server
// shutting down, and a service that panics leaves its connection closed, as
// the server's failure.
func TestStreamHandlerStreamsTheResponsesOfARequest(t *testing.T) {
	target, closeCodes := streamServer(t, watchRoute)

	conn := dialStream(t, target)
	require.NoError(t, conn.Send([]byte(`{"note":"3"}`)))
	for i := range 3 {
		rsp, err := nextMessage(t, conn)
		require.NoError(t, err)
		require.Equal(t, strconv.Itoa(i), rsp["Note"])
		require.Equal(t, "alice", rsp["Username"])
		require.Equal(t, "b-3", rsp["Box"])
		require.Equal(t, "/api/boxes/:box/"+watchRoute, rsp["Route"])
		require.Equal(t, http.MethodGet, rsp["Method"])
	}
	_, err := nextMessage(t, conn)
	require.ErrorIs(t, err, io.EOF)
	require.Equal(t, 1000, awaitCloseCode(t, closeCodes))

	refusals := []struct {
		name    string
		request string
		status  int
		msg     string
	}{
		{name: "a request failing validation closes the stream as the refusal", request: `{}`, status: http.StatusBadRequest, msg: "note is a required field"},
		{name: "a request that is no JSON closes the stream as the refusal", request: `{"note":`, status: http.StatusBadRequest, msg: "request body is not valid JSON"},
		{name: "the service's error closes the stream with its status", request: `{"note":"` + actionRefuse + `"}`, status: http.StatusForbidden, msg: "not yours"},
		{name: "any other error closes the stream as the server's failure", request: `{"note":"` + actionBreak + `"}`, status: http.StatusInternalServerError, msg: types.FailureMsg},
	}
	for _, tt := range refusals {
		t.Run(tt.name, func(t *testing.T) {
			conn := dialStream(t, target)
			require.NoError(t, conn.Send([]byte(tt.request)))
			_, err := nextMessage(t, conn)
			requireEnding(t, err, tt.status, tt.msg)
			require.Equal(t, 4000+tt.status, awaitCloseCode(t, closeCodes))
		})
	}

	t.Run("a client ending its messages after the request is taken at its word", func(t *testing.T) {
		conn := dialStream(t, target)
		require.NoError(t, conn.Send([]byte(`{"note":"1"}`)))
		require.NoError(t, conn.CloseSend())
		rsp, err := nextMessage(t, conn)
		require.NoError(t, err)
		require.Equal(t, "0", rsp["Note"])
		_, err = nextMessage(t, conn)
		require.ErrorIs(t, err, io.EOF)
		require.Equal(t, 1000, awaitCloseCode(t, closeCodes))
	})

	t.Run("a client ending its messages without a request sends the zero request", func(t *testing.T) {
		conn := dialStream(t, target)
		require.NoError(t, conn.CloseSend())
		_, err := nextMessage(t, conn)
		requireEnding(t, err, http.StatusBadRequest, "note is a required field")
		require.Equal(t, 4400, awaitCloseCode(t, closeCodes))
	})

	t.Run("a message after the request closes the stream as a violation", func(t *testing.T) {
		conn := dialStream(t, target)
		require.NoError(t, conn.Send([]byte(`{"note":"`+actionAwait+`"}`)))
		awaitService(t)
		require.NoError(t, conn.Send([]byte(`{"note":"1"}`)))
		_, err := nextMessage(t, conn)
		require.Error(t, err)
		require.Equal(t, 1008, awaitCloseCode(t, closeCodes))
	})

	t.Run("a request that does not arrive in time closes the stream as a 408", func(t *testing.T) {
		original := config.App.Server.ReadTimeout
		config.App.Server.ReadTimeout = 50 * time.Millisecond
		t.Cleanup(func() { config.App.Server.ReadTimeout = original })

		conn := dialStream(t, target)
		_, err := nextMessage(t, conn)
		requireEnding(t, err, http.StatusRequestTimeout, "the request of the stream did not arrive in time")
		require.Equal(t, 4408, awaitCloseCode(t, closeCodes))
	})

	t.Run("a client leaving ends the stream", func(t *testing.T) {
		conn := dialStream(t, target)
		require.NoError(t, conn.Send([]byte(`{"note":"`+actionAwait+`"}`)))
		awaitService(t)
		conn.Close(nil)
		require.Equal(t, 1000, awaitCloseCode(t, closeCodes), "the status the client closed with, the service having returned once its context ended")
	})

	t.Run("the server shutting down ends the stream as going away", func(t *testing.T) {
		shutdown, beginShutdown := context.WithCancel(context.Background())
		defer beginShutdown()
		target, closeCodes := streamServer(t, watchRoute, func(c *gin.Context) {
			c.Request = c.Request.WithContext(streamctx.WithServerShutdown(c.Request.Context(), shutdown))
		})

		conn := dialStream(t, target)
		require.NoError(t, conn.Send([]byte(`{"note":"`+actionAwait+`"}`)))
		awaitService(t)
		beginShutdown()
		_, err := nextMessage(t, conn)
		requireEnding(t, err, http.StatusServiceUnavailable, streamctx.ShutdownMsg)
		require.Equal(t, 1001, awaitCloseCode(t, closeCodes))
	})

	t.Run("a service that panics leaves the stream closed as the server's failure", func(t *testing.T) {
		// The recovery of the listener's chain, which takes the panic once
		// the handler has unwound.
		target, closeCodes := streamServer(t, watchRoute, func(c *gin.Context) {
			defer func() { _ = recover() }()
			c.Next()
		})

		conn := dialStream(t, target)
		require.NoError(t, conn.Send([]byte(`{"note":"`+actionPanic+`"}`)))
		_, err := nextMessage(t, conn)
		requireEnding(t, err, http.StatusInternalServerError, types.FailureMsg)
		require.Equal(t, 4500, awaitCloseCode(t, closeCodes))
	})
}

// TestStreamHandlerAnswersAStreamOfRequests pins the client stream over HTTP:
// the service reads the client's messages one request each until the frame
// that ends them, its response is the one message the server sends, and the
// connection closes normally after it; a request the validator refuses
// closes the stream as the 400 it is, whatever the service returns for the
// error its Recv gave it.
func TestStreamHandlerAnswersAStreamOfRequests(t *testing.T) {
	target, closeCodes := streamServer(t, uploadRoute)

	conn := dialStream(t, target)
	require.NoError(t, conn.Send([]byte(`{"note":"a"}`)))
	require.NoError(t, conn.Send([]byte(`{"note":"b"}`)))
	require.NoError(t, conn.CloseSend())
	rsp, err := nextMessage(t, conn)
	require.NoError(t, err)
	require.Equal(t, "a,b", rsp["Note"])
	require.Equal(t, "alice", rsp["Username"])
	_, err = nextMessage(t, conn)
	require.ErrorIs(t, err, io.EOF)
	require.Equal(t, 1000, awaitCloseCode(t, closeCodes))

	t.Run("a request failing validation closes the stream as the refusal", func(t *testing.T) {
		conn := dialStream(t, target)
		require.NoError(t, conn.Send([]byte(`{"note":"a"}`)))
		require.NoError(t, conn.Send([]byte(`{}`)))
		_, err := nextMessage(t, conn)
		requireEnding(t, err, http.StatusBadRequest, "note is a required field")
		require.Equal(t, 4400, awaitCloseCode(t, closeCodes))
	})
}

// TestStreamHandlerStreamsBothWays pins the bidirectional stream over HTTP:
// each request the client sends is answered as it comes, and the connection
// closes normally once the client has ended its messages and the service
// returned.
func TestStreamHandlerStreamsBothWays(t *testing.T) {
	target, closeCodes := streamServer(t, chatRoute)

	conn := dialStream(t, target)
	for _, note := range []string{"a", "b"} {
		require.NoError(t, conn.Send([]byte(`{"note":"`+note+`"}`)))
		rsp, err := nextMessage(t, conn)
		require.NoError(t, err)
		require.Equal(t, "echo "+note, rsp["Note"])
	}
	require.NoError(t, conn.CloseSend())
	_, err := nextMessage(t, conn)
	require.ErrorIs(t, err, io.EOF)
	require.Equal(t, 1000, awaitCloseCode(t, closeCodes))
}

// TestStreamHandlerKeepsARefusalWhateverTheServiceReturns pins that a request
// the validator refuses closes the stream as the refusal though the service
// returns no error for it: the fixture reads on a goroutine of its own and
// returns nil without waiting for what the read got.
func TestStreamHandlerKeepsARefusalWhateverTheServiceReturns(t *testing.T) {
	target, closeCodes := streamServer(t, forkRoute)

	conn := dialStream(t, target)
	require.NoError(t, conn.Send([]byte(`{}`)))
	_, err := nextMessage(t, conn)
	requireEnding(t, err, http.StatusBadRequest, "note is a required field")
	require.Equal(t, 4400, awaitCloseCode(t, closeCodes))
	select {
	case got := <-forkingChat.read:
		requireEnding(t, got, http.StatusBadRequest, "note is a required field")
	case <-time.After(5 * time.Second):
		t.Fatal("the goroutine's read did not end")
	}
}

// TestStreamHandlerAnswersARequestItDoesNotOpenAsAnEnvelope pins what the
// handler answers before the connection is open, a regular error envelope
// each: a request that is no handshake and one from an origin not admitted,
// with the status and the text of the refusal, and a service declaring no
// Stream method of a kind, with 501.
func TestStreamHandlerAnswersARequestItDoesNotOpenAsAnEnvelope(t *testing.T) {
	target, _ := streamServer(t, watchRoute)
	silent, _ := streamServer(t, silentRoute)
	msgOf := func(t *testing.T, rsp *http.Response) string {
		t.Helper()
		var envelope struct {
			Msg string `json:"msg"`
		}
		require.NoError(t, json.NewDecoder(rsp.Body).Decode(&envelope))
		return envelope.Msg
	}

	t.Run("a request that is no handshake", func(t *testing.T) {
		rsp, err := http.Get(target)
		require.NoError(t, err)
		defer rsp.Body.Close()
		require.Equal(t, http.StatusUpgradeRequired, rsp.StatusCode)
		require.Equal(t, "websocket", rsp.Header.Get("Upgrade"))
		require.Equal(t, `WebSocket protocol violation: Connection header "" does not contain Upgrade`, msgOf(t, rsp))
	})

	t.Run("an origin not admitted", func(t *testing.T) {
		_, rsp, err := websocket.Dial(t.Context(), nil, target, http.Header{"Origin": {"https://app.other.example"}}, time.Second)
		require.Error(t, err)
		require.Equal(t, http.StatusForbidden, rsp.StatusCode)
		address, err := url.Parse(target)
		require.NoError(t, err)
		require.Equal(t, `request Origin "app.other.example" is not authorized for Host "`+address.Host+`"`, msgOf(t, rsp))
	})

	t.Run("a service declaring no Stream method", func(t *testing.T) {
		_, rsp, err := websocket.Dial(t.Context(), nil, silent, nil, time.Second)
		require.Error(t, err)
		require.Equal(t, http.StatusNotImplemented, rsp.StatusCode)
		require.Equal(t, "the Stream action is served by no Stream method of its kind", msgOf(t, rsp))
	})
}

// TestTransportsCarryAStreamAlike pins the contract of the Stream action on
// both transports at once: every scenario is sent as the messages of a
// WebSocket connection and as the messages of the gRPC stream, the same
// requests on both, and both must deliver the same responses and end alike —
// with the HTTP status the scenario states, which the status the connection
// closes with stands for over HTTP and the gRPC code it maps to answers over
// gRPC (see grpcCodeOf), and with the same msg. What the transports differ on
// by design is stated beside the scenarios (see TRANSPORTS.md, section 6).
func TestTransportsCarryAStreamAlike(t *testing.T) {
	grpcConn := sampleServer(t)
	note := func(notes ...string) []map[string]any {
		requests := make([]map[string]any, 0, len(notes))
		for _, n := range notes {
			requests = append(requests, map[string]any{"note": n})
		}
		return requests
	}
	serverStream := grpc.StreamDesc{ServerStreams: true}
	clientStream := grpc.StreamDesc{ClientStreams: true}
	bidiStream := grpc.StreamDesc{ServerStreams: true, ClientStreams: true}
	const required = "note is a required field"

	scenarios := []struct {
		name     string
		route    string
		rpc      string
		desc     grpc.StreamDesc
		requests []map[string]any
		// notes are the notes of the responses, in order, and status and
		// msg how the stream ends, 200 for an ending without an error.
		notes  []string
		status int
		msg    string
	}{
		{name: "a server stream streams its responses", route: watchRoute, rpc: "Watch", desc: serverStream, requests: note("2"), notes: []string{"0", "1"}, status: http.StatusOK},
		{name: "a server stream refuses a request failing validation", route: watchRoute, rpc: "Watch", desc: serverStream, requests: []map[string]any{{}}, status: http.StatusBadRequest, msg: required},
		{name: "a server stream ends with its service's error", route: watchRoute, rpc: "Watch", desc: serverStream, requests: note(actionRefuse), status: http.StatusForbidden, msg: "not yours"},
		{name: "a server stream ends with the server's failure", route: watchRoute, rpc: "Watch", desc: serverStream, requests: note(actionBreak), status: http.StatusInternalServerError, msg: types.FailureMsg},
		{name: "a client stream answers its requests", route: uploadRoute, rpc: "Upload", desc: clientStream, requests: note("a", "b"), notes: []string{"a,b"}, status: http.StatusOK},
		{name: "a client stream refuses a request failing validation", route: uploadRoute, rpc: "Upload", desc: clientStream, requests: []map[string]any{{"note": "a"}, {}}, status: http.StatusBadRequest, msg: required},
		{name: "a bidirectional stream answers each request", route: chatRoute, rpc: "Chat", desc: bidiStream, requests: note("a", "b"), notes: []string{"echo a", "echo b"}, status: http.StatusOK},
		{name: "a bidirectional stream refuses a request failing validation", route: chatRoute, rpc: "Chat", desc: bidiStream, requests: []map[string]any{{"note": "a"}, {}}, notes: []string{"echo a"}, status: http.StatusBadRequest, msg: required},
		// Over HTTP the action nothing serves is refused before the
		// connection opens, as a regular envelope; over gRPC as the status
		// of the stream.
		{name: "a service declaring no Stream method serves no stream", route: silentRoute, rpc: "Silence", desc: serverStream, requests: note("1"), status: http.StatusNotImplemented, msg: "the Stream action is served by no Stream method of its kind"},
	}
	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			t.Run("HTTP", func(t *testing.T) {
				target, _ := streamServer(t, s.route)
				conn, rsp, err := websocket.Dial(t.Context(), nil, target, nil, time.Second)
				if err != nil {
					var envelope struct {
						Msg string `json:"msg"`
					}
					require.NoError(t, json.NewDecoder(rsp.Body).Decode(&envelope))
					require.Empty(t, s.notes)
					require.Equal(t, s.status, rsp.StatusCode)
					require.Equal(t, s.msg, envelope.Msg)
					return
				}
				t.Cleanup(func() { conn.Close(nil) })
				for _, request := range s.requests {
					message, err := json.Marshal(request)
					require.NoError(t, err)
					require.NoError(t, conn.Send(message))
				}
				// The client is through, the way the gRPC client half-closes.
				_ = conn.CloseSend()
				var notes []string
				for {
					rsp, err := nextMessage(t, conn)
					if errors.Is(err, io.EOF) {
						require.Equal(t, http.StatusOK, s.status)
						break
					}
					if err != nil {
						requireEnding(t, err, s.status, s.msg)
						break
					}
					notes = append(notes, fmt.Sprint(rsp["Note"]))
					// The transports differ by design on the verb a service
					// reads off its context: the GET of the handshake over
					// HTTP, STREAM over gRPC.
					require.Equal(t, http.MethodGet, rsp["Method"])
				}
				require.Equal(t, s.notes, notes)
			})

			t.Run("gRPC", func(t *testing.T) {
				stream := openStream(t, grpcConn, s.rpc, sampleCredential, s.desc)
				for _, request := range s.requests {
					// A send may fail once the stream has been refused; the
					// answer read below is what is held to the scenario.
					_ = stream.SendMsg(encode(map[string]any{"payload": request}))
				}
				_ = stream.CloseSend()
				var notes []string
				for {
					rsp, err := recvResponse(stream)
					if errors.Is(err, io.EOF) {
						require.Equal(t, http.StatusOK, s.status)
						break
					}
					if err != nil {
						st := status.Convert(err)
						require.Equal(t, grpcCodeOf(s.status), st.Code(), st.Message())
						require.Equal(t, s.msg, st.Message())
						break
					}
					notes = append(notes, fmt.Sprint(rsp["Note"]))
					require.Equal(t, grpcserver.MethodStream, rsp["Method"])
				}
				require.Equal(t, s.notes, notes)
			})
		})
	}
	require.Equal(t, codes.Unimplemented, grpcCodeOf(http.StatusNotImplemented), "the code a stream nothing serves answers with")
}

// streamServer serves the Stream action of the fixture service on route over
// HTTP, on a server of its own, the way serveAs serves a request: at
// /api/boxes/:box/<route>, the route parameters named and the caller alice,
// behind chain. It returns the address of the action for the box b-3 and the
// status each of its connections closed with, as the handler leaves it for
// the access log.
func streamServer(t *testing.T, route string, chain ...gin.HandlerFunc) (string, <-chan int) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	pattern := "/api/boxes/:box/" + route
	middleware.RouteManager.Add(pattern)
	closeCodes := make(chan int, 8)
	engine := gin.New()
	engine.Use(chain...)
	engine.GET(pattern, func(c *gin.Context) {
		c.Set(requestctx.KeyParams, middleware.RouteManager.Get(c.FullPath()))
		c.Set(requestctx.KeyUsername, "alice")
		c.Set(requestctx.KeyUserID, "u-1")
		// Deferred, so that it is read after a handler that panicked too.
		defer func() { closeCodes <- c.GetInt(requestctx.KeyCloseCode) }()
		c.Next()
	}, controller.StreamHandler[*sampleRecord, *sampleActionReq, *sampleActionRsp](&types.ControllerConfig[*sampleRecord]{Route: route}))
	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)
	return srv.URL + "/api/boxes/b-3/" + route, closeCodes
}

// dialStream opens the stream at target and closes it when the test ends.
func dialStream(t *testing.T, target string) *websocket.Conn {
	t.Helper()

	conn, _, err := websocket.Dial(t.Context(), nil, target, nil, time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close(nil) })
	return conn
}

// nextMessage returns the next message of the stream conn carries, decoded,
// or the error the stream ended with for its client, and fails the test when
// neither arrives in time.
func nextMessage(t *testing.T, conn *websocket.Conn) (map[string]any, error) {
	t.Helper()

	type answer struct {
		message []byte
		err     error
	}
	answers := make(chan answer, 1)
	go func() {
		message, err := conn.Recv()
		answers <- answer{message: message, err: err}
	}()
	select {
	case got := <-answers:
		if got.err != nil {
			return nil, got.err
		}
		var message map[string]any
		require.NoError(t, json.Unmarshal(got.message, &message))
		return message, nil
	case <-time.After(10 * time.Second):
		t.Fatal("the stream answered nothing in time")
		panic("unreachable")
	}
}

// requireEnding requires err, what a stream over HTTP ended with for its
// client, to be the service error of status carrying msg, which the status
// the connection closed with stands for.
func requireEnding(t *testing.T, err error, status int, msg string) {
	t.Helper()

	var ending *types.Error
	require.ErrorAs(t, err, &ending)
	require.Equal(t, status, ending.Status())
	require.Equal(t, msg, ending.Msg())
}

// awaitCloseCode returns the status the next connection closed with, and
// fails the test when none closes in time.
func awaitCloseCode(t *testing.T, closeCodes <-chan int) int {
	t.Helper()

	select {
	case code := <-closeCodes:
		return code
	case <-time.After(10 * time.Second):
		t.Fatal("no connection closed in time")
		return 0
	}
}

// awaitService waits for a stream service to have read its request and to
// wait for its client (see actionAwait).
func awaitService(t *testing.T) {
	t.Helper()

	select {
	case <-awaitEntered:
	case <-time.After(10 * time.Second):
		t.Fatal("the service did not start waiting for the client")
	}
}
