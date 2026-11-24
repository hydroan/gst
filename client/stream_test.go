package client_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/streamctx"
	"github.com/hydroan/gst/internal/websocket"
	"github.com/stretchr/testify/require"
)

// sampleNote is the message the streams of these tests carry, a request or a
// response.
type sampleNote struct {
	Note string `json:"note"`
}

// TestServerStreamReceivesTheResponsesOfItsRequest pins the server stream:
// the handshake carries what a request of the client carries, its query
// parameters and its credentials, the payload arrives as the one request of
// the stream, and Recv returns the responses in order, io.EOF once the
// server has ended the stream; a stream opened without a payload sends no
// request and starts at once.
func TestServerStreamReceivesTheResponsesOfItsRequest(t *testing.T) {
	type handshake struct{ authorization, query string }
	handshakes := make(chan handshake, 1)
	cli := streamClient(t, func(conn *websocket.Conn, r *http.Request) error {
		handshakes <- handshake{authorization: r.Header.Get("Authorization"), query: r.URL.RawQuery}
		request, err := conn.Recv()
		if errors.Is(err, io.EOF) {
			return conn.Send([]byte(`{"note":"no request"}`))
		}
		if err != nil {
			return err
		}
		for range 2 {
			if err = conn.Send(request); err != nil {
				return err
			}
		}
		return nil
	}, client.WithToken("sample-token"))

	stream, err := cli.ServerStream[sampleNote](streamContext(t), "/api/samples/watch", &sampleNote{Note: "watched"}, client.WithQuery("box", "b-3"))
	require.NoError(t, err)
	defer stream.Close()
	require.Equal(t, handshake{authorization: "Bearer sample-token", query: "box=b-3"}, <-handshakes)
	for range 2 {
		rsp, recvErr := stream.Recv()
		require.NoError(t, recvErr)
		require.Equal(t, "watched", rsp.Note)
	}
	_, err = stream.Recv()
	require.ErrorIs(t, err, io.EOF)

	t.Run("without a payload the stream sends no request", func(t *testing.T) {
		stream, err := cli.ServerStream[sampleNote](streamContext(t), "/api/samples/watch", nil)
		require.NoError(t, err)
		defer stream.Close()
		<-handshakes
		rsp, err := stream.Recv()
		require.NoError(t, err)
		require.Equal(t, "no request", rsp.Note)
	})
}

// TestClientStreamSendsItsRequestsAndReceivesTheResponse pins the client
// stream: Send sends the requests one by one, and CloseAndRecv tells the
// server they are through and returns its response, the zero response when
// the server ends the stream without one, as the fixture does for a stream
// whose first note is silentNote.
func TestClientStreamSendsItsRequestsAndReceivesTheResponse(t *testing.T) {
	const silentNote = "silent"
	cli := streamClient(t, func(conn *websocket.Conn, _ *http.Request) error {
		var notes []string
		for {
			request, err := conn.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return err
			}
			var note sampleNote
			if err = json.Unmarshal(request, &note); err != nil {
				return err
			}
			notes = append(notes, note.Note)
		}
		if len(notes) > 0 && notes[0] == silentNote {
			return nil
		}
		response, err := json.Marshal(sampleNote{Note: strings.Join(notes, ",")})
		if err != nil {
			return err
		}
		return conn.Send(response)
	})

	stream, err := cli.ClientStream[sampleNote, sampleNote](streamContext(t), "/api/samples/upload")
	require.NoError(t, err)
	defer stream.Close()
	require.NoError(t, stream.Send(&sampleNote{Note: "a"}))
	require.NoError(t, stream.Send(&sampleNote{Note: "b"}))
	rsp, err := stream.CloseAndRecv()
	require.NoError(t, err)
	require.Equal(t, "a,b", rsp.Note)

	t.Run("a stream the server ends without a response answers the zero response", func(t *testing.T) {
		stream, err := cli.ClientStream[sampleNote, sampleNote](streamContext(t), "/api/samples/upload")
		require.NoError(t, err)
		defer stream.Close()
		require.NoError(t, stream.Send(&sampleNote{Note: silentNote}))
		rsp, err := stream.CloseAndRecv()
		require.NoError(t, err)
		require.Equal(t, &sampleNote{}, rsp)
	})
}

// TestBidiStreamExchangesItsMessages pins the bidirectional stream: each
// request is answered as it is sent, CloseSend tells the server the requests
// are through, and Recv answers io.EOF once the server has ended the stream.
func TestBidiStreamExchangesItsMessages(t *testing.T) {
	cli := streamClient(t, func(conn *websocket.Conn, _ *http.Request) error {
		for {
			request, err := conn.Recv()
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return err
			}
			if err = conn.Send(request); err != nil {
				return err
			}
		}
	})

	stream, err := cli.BidiStream[sampleNote, sampleNote](streamContext(t), "/api/samples/chat")
	require.NoError(t, err)
	defer stream.Close()
	for _, note := range []string{"a", "b"} {
		require.NoError(t, stream.Send(&sampleNote{Note: note}))
		rsp, recvErr := stream.Recv()
		require.NoError(t, recvErr)
		require.Equal(t, note, rsp.Note)
	}
	require.NoError(t, stream.CloseSend())
	_, err = stream.Recv()
	require.ErrorIs(t, err, io.EOF)
}

// TestStreamSurfacesARefusedHandshakeAsARejection proves a handshake the
// server refuses surfaces the way the rejection of any request does, as the
// *Error of the envelope the server answered, whole however long it is.
func TestStreamSurfacesARefusedHandshakeAsARejection(t *testing.T) {
	msg := strings.Repeat("the caller may not open this stream; ", 60)
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"msg": msg, "trace_id": "t-1"})
	}))
	srv.Start()
	cli, err := client.New(srv.URL)
	require.NoError(t, err)

	stream, err := cli.BidiStream[sampleNote, sampleNote](streamContext(t), "/api/samples/chat")
	require.Nil(t, stream)
	var refused *client.Error
	require.ErrorAs(t, err, &refused)
	require.Equal(t, http.StatusForbidden, refused.StatusCode)
	require.Equal(t, msg, refused.Msg)
	require.Equal(t, "t-1", refused.TraceID)
}

// TestStreamSurfacesTheErrorTheServerEndedItWith proves a stream the server
// ends with an error surfaces as the *Error of the status and the message it
// was ended with: a service error, the server's own failure, and the server
// shutting down.
func TestStreamSurfacesTheErrorTheServerEndedItWith(t *testing.T) {
	tests := []struct {
		name   string
		ending error
		status int
		msg    string
	}{
		{name: "a service error", ending: gst.NewError(http.StatusNotFound, "sample not found"), status: http.StatusNotFound, msg: "sample not found"},
		{name: "another error", ending: errors.New("sample failure"), status: http.StatusInternalServerError, msg: gst.FailureMsg},
		{name: "the server shutting down", ending: streamctx.ErrServerShutdown, status: http.StatusServiceUnavailable, msg: streamctx.ShutdownMsg},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli := streamClient(t, func(*websocket.Conn, *http.Request) error { return tt.ending })

			stream, err := cli.BidiStream[sampleNote, sampleNote](streamContext(t), "/api/samples/chat")
			require.NoError(t, err)
			defer stream.Close()
			_, err = stream.Recv()
			var ended *client.Error
			require.ErrorAs(t, err, &ended)
			require.Equal(t, tt.status, ended.StatusCode)
			require.Equal(t, tt.msg, ended.Msg)
		})
	}
}

// TestStreamEndsWhenTheClientEndsIt proves the two ways a client ends a
// stream the server would keep open: the context it was opened on ending,
// its methods then failing with the context's error, and Close. Either way
// the server sees its client leave.
func TestStreamEndsWhenTheClientEndsIt(t *testing.T) {
	left := make(chan struct{}, 1)
	cli := streamClient(t, func(conn *websocket.Conn, _ *http.Request) error {
		<-conn.Context().Done()
		left <- struct{}{}
		return nil
	})
	requireLeft := func(t *testing.T) {
		t.Helper()
		select {
		case <-left:
		case <-time.After(5 * time.Second):
			t.Fatal("the server must see the client leave")
		}
	}

	t.Run("its context ending", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		stream, err := cli.BidiStream[sampleNote, sampleNote](ctx, "/api/samples/chat")
		require.NoError(t, err)
		defer stream.Close()
		cancel()
		received := make(chan error, 1)
		go func() {
			_, recvErr := stream.Recv()
			received <- recvErr
		}()
		select {
		case recvErr := <-received:
			require.ErrorIs(t, recvErr, context.Canceled)
		case <-time.After(5 * time.Second):
			t.Fatal("Recv must fail once the context the stream was opened on has ended")
		}
		require.ErrorIs(t, stream.Send(&sampleNote{Note: "late"}), context.Canceled)
		requireLeft(t)
	})

	t.Run("Close", func(t *testing.T) {
		stream, err := cli.BidiStream[sampleNote, sampleNote](streamContext(t), "/api/samples/chat")
		require.NoError(t, err)
		require.NoError(t, stream.Close())
		requireLeft(t)
	})
}

// streamClient returns a client of a server that accepts every request as a
// stream and hands its connection to serve, closing it with what serve
// returns, the way the handler of a Stream action ends its stream.
func streamClient(t *testing.T, serve func(conn *websocket.Conn, r *http.Request) error, opts ...client.Option) *client.Client {
	t.Helper()

	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil, time.Second)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		conn.Close(serve(conn, r))
	}))
	srv.Start()
	cli, err := client.New(srv.URL, opts...)
	require.NoError(t, err)
	return cli
}

// streamContext returns the context a test opens a stream on: bound, so
// that a stream nothing ends fails its test instead of holding it.
func streamContext(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}
