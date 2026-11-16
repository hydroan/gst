package websocket_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	coderwebsocket "github.com/coder/websocket"
	"github.com/hydroan/gst/internal/streamctx"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/internal/websocket"
	"github.com/stretchr/testify/require"
)

// TestConnsExchangeMessagesUntilEachSideFinishes proves the two ends of a
// connection carry messages both ways and tell each other when they are
// through: the client's CloseSend ends the server's Recv with io.EOF, the
// server still sends after it, and the server closing the connection without
// an error ends the client's Recv with io.EOF.
func TestConnsExchangeMessagesUntilEachSideFinishes(t *testing.T) {
	served := make(chan error, 1)
	url := serve(t, func(w http.ResponseWriter, r *http.Request) {
		served <- func() error {
			conn, err := websocket.Accept(w, r, nil, time.Second)
			if err != nil {
				return err
			}
			defer conn.Close(nil)
			for {
				message, err := conn.Recv()
				if errors.Is(err, io.EOF) {
					return conn.Send([]byte(`{"finished":true}`))
				}
				if err != nil {
					return err
				}
				if err := conn.Send(message); err != nil {
					return err
				}
			}
		}()
	})

	conn := dial(t, url, nil)
	for _, message := range []string{`{"n":1}`, `{"n":2}`} {
		require.NoError(t, conn.Send([]byte(message)))
		echoed, err := recv(t, conn)
		require.NoError(t, err)
		require.JSONEq(t, message, string(echoed))
	}
	require.NoError(t, conn.CloseSend())
	last, err := recv(t, conn)
	require.NoError(t, err)
	require.JSONEq(t, `{"finished":true}`, string(last))
	_, err = recv(t, conn)
	require.ErrorIs(t, err, io.EOF)
	require.NoError(t, await(t, served))
}

// TestAcceptHoldsBackTheRefusalOfAHandshake proves Accept writes nothing when
// the handshake is refused: it returns the refusal as an error carrying the
// status and the text, for its caller to answer the way it answers every
// refusal, and passes on the headers the protocol answers such a request
// with.
func TestAcceptHoldsBackTheRefusalOfAHandshake(t *testing.T) {
	// refusing starts a server that answers a handshake Accept refuses with a
	// status of its own, and reports what Accept returned.
	refusing := func(t *testing.T) (string, <-chan error) {
		t.Helper()

		refused := make(chan error, 1)
		url := serve(t, func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, nil, time.Second)
			if err != nil {
				refused <- err
				w.WriteHeader(http.StatusTeapot)
				return
			}
			conn.Close(nil)
			refused <- nil
		})
		return url, refused
	}

	t.Run("a request that is no handshake", func(t *testing.T) {
		url, refused := refusing(t)
		rsp, err := http.Get(url)
		require.NoError(t, err)
		defer rsp.Body.Close()
		require.Equal(t, http.StatusTeapot, rsp.StatusCode, "the answer is the caller's to write")
		require.Equal(t, "websocket", rsp.Header.Get("Upgrade"))

		var refusal *types.Error
		require.ErrorAs(t, await(t, refused), &refusal)
		require.Equal(t, http.StatusUpgradeRequired, refusal.Status())
		require.Equal(t, `WebSocket protocol violation: Connection header "" does not contain Upgrade`, refusal.Msg())
	})

	t.Run("an origin of another host", func(t *testing.T) {
		url, refused := refusing(t)
		host := strings.TrimPrefix(url, "http://")
		_, rsp, err := websocket.Dial(t.Context(), nil, url, http.Header{"Origin": {"https://app.other.example"}}, time.Second)
		require.Error(t, err)
		require.Equal(t, http.StatusTeapot, rsp.StatusCode, "the answer is the caller's to write")

		var refusal *types.Error
		require.ErrorAs(t, await(t, refused), &refusal)
		require.Equal(t, http.StatusForbidden, refusal.Status())
		require.Equal(t, `request Origin "app.other.example" is not authorized for Host "`+host+`"`, refusal.Msg())
	})
}

// TestDialKeepsTheWholeAnswerOfAHandshakeRefused proves Dial returns the
// answer of a handshake the server refuses whole, where the library keeps
// its first kilobyte: the status, and a body longer than that.
func TestDialKeepsTheWholeAnswerOfAHandshakeRefused(t *testing.T) {
	answer := strings.Repeat("a", 5000)
	url := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, answer)
	})

	_, rsp, err := websocket.Dial(t.Context(), nil, url, nil, time.Second)
	require.Error(t, err)
	require.Equal(t, http.StatusForbidden, rsp.StatusCode)
	body, err := io.ReadAll(rsp.Body)
	require.NoError(t, err)
	require.Equal(t, answer, string(body))
}

// TestAcceptAdmitsTheOriginsListed proves which pages may open a connection:
// a request carrying no origin, the pages of the host the request is sent to,
// and those of the origins listed, by host or by scheme and host.
func TestAcceptAdmitsTheOriginsListed(t *testing.T) {
	url := serve(t, func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, []string{"*.other.example", "https://partner.example"}, time.Second)
		if err != nil {
			w.WriteHeader(http.StatusTeapot)
			return
		}
		conn.Close(nil)
	})

	for name, origin := range map[string]string{
		"no origin":                      "",
		"the host the request is to":     url,
		"an origin listed by its host":   "https://app.other.example",
		"an origin listed with a scheme": "https://partner.example",
	} {
		t.Run(name, func(t *testing.T) {
			header := http.Header{}
			if origin != "" {
				header.Set("Origin", origin)
			}
			_, err := recv(t, dial(t, url, header))
			require.ErrorIs(t, err, io.EOF)
		})
	}
}

// TestConnReadsAMessageOfTheLargestSize pins the largest message a
// connection reads, 4 MiB: one of exactly that size arrives whole.
func TestConnReadsAMessageOfTheLargestSize(t *testing.T) {
	received := make(chan int, 1)
	url := serve(t, func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil, time.Second)
		if err != nil {
			return
		}
		defer conn.Close(nil)
		message, _ := conn.Recv()
		received <- len(message)
	})

	conn := dial(t, url, nil)
	require.NoError(t, conn.Send(messageOfSize(4<<20)))
	require.Equal(t, 4<<20, await(t, received))
}

// TestConnClosesOnWhatIsNoMessage proves a connection closes, with the status
// the protocol gives each, on what a client may not send: a binary message, a
// message over the largest size by one byte, and a message after the one that
// ended the client's.
func TestConnClosesOnWhatIsNoMessage(t *testing.T) {
	tests := []struct {
		name   string
		send   func(ctx context.Context, client *coderwebsocket.Conn) error
		status coderwebsocket.StatusCode
	}{
		{
			name: "a binary message",
			send: func(ctx context.Context, client *coderwebsocket.Conn) error {
				return client.Write(ctx, coderwebsocket.MessageBinary, []byte(`{"n":1}`))
			},
			status: coderwebsocket.StatusUnsupportedData,
		},
		{
			name: "a message over the largest size",
			send: func(ctx context.Context, client *coderwebsocket.Conn) error {
				return client.Write(ctx, coderwebsocket.MessageText, messageOfSize(4<<20+1))
			},
			status: coderwebsocket.StatusMessageTooBig,
		},
		{
			name: "a message after the last",
			send: func(ctx context.Context, client *coderwebsocket.Conn) error {
				if err := client.Write(ctx, coderwebsocket.MessageText, []byte("null")); err != nil {
					return err
				}
				return client.Write(ctx, coderwebsocket.MessageText, []byte(`{"n":1}`))
			},
			status: coderwebsocket.StatusPolicyViolation,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ended := make(chan error, 1)
			url := serve(t, func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil, time.Second)
				if err != nil {
					ended <- err
					return
				}
				defer conn.Close(nil)
				<-conn.Context().Done()
				ended <- context.Cause(conn.Context())
			})

			client := dialLibrary(t, url)
			// The write may fail once the server has closed the connection on
			// it; the status the connection closed with is what is asserted.
			_ = tt.send(t.Context(), client)
			err := closedWith(t, client)
			require.Equal(t, tt.status, coderwebsocket.CloseStatus(err), "closed with: %v", err)
			require.Error(t, await(t, ended), "the connection's context ends with what closed it")
		})
	}
}

// TestCloseAnswersWithTheStatusOfTheEnding pins the status and the reason an
// accepted connection closes with for each ending of a stream: none, a
// service error, whose message is cut to what a close frame holds without
// splitting a character, any other error, and the server shutting down.
func TestCloseAnswersWithTheStatusOfTheEnding(t *testing.T) {
	tests := []struct {
		name   string
		ending error
		status coderwebsocket.StatusCode
		reason string
	}{
		{name: "no error", ending: nil, status: coderwebsocket.StatusNormalClosure, reason: ""},
		{name: "a service error", ending: types.NewError(http.StatusNotFound, "sample not found"), status: 4404, reason: "sample not found"},
		{name: "a service error with a long message", ending: types.NewError(http.StatusBadRequest, strings.Repeat("é", 100)), status: 4400, reason: strings.Repeat("é", 61)},
		{name: "another error", ending: errors.New("sample failure"), status: 4500, reason: types.FailureMsg},
		{name: "the server shutting down", ending: errors.Wrap(streamctx.ErrServerShutdown, "sample stream"), status: coderwebsocket.StatusGoingAway, reason: "the server is shutting down"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := serve(t, func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil, time.Second)
				if err != nil {
					return
				}
				conn.Close(tt.ending)
			})

			var closed coderwebsocket.CloseError
			require.ErrorAs(t, closedWith(t, dialLibrary(t, url)), &closed)
			require.Equal(t, tt.status, closed.Code)
			require.Equal(t, tt.reason, closed.Reason)
		})
	}
}

// TestCloseRecvEndsWhatTheClientMaySend proves a connection whose server calls
// CloseRecv hands out no more messages, Recv answering io.EOF, and closes,
// with StatusPolicyViolation, on a message its client sends from then on or
// had sent and Recv had not taken, while the frame that ends the client's
// messages is still taken.
func TestCloseRecvEndsWhatTheClientMaySend(t *testing.T) {
	tests := []struct {
		name string
		// before and after are what the client sends beside its first
		// message, before the server calls CloseRecv and after.
		before, after string
		status        coderwebsocket.StatusCode
	}{
		{name: "a message sent after it", after: `{"n":2}`, status: coderwebsocket.StatusPolicyViolation},
		{name: "a message sent before it that Recv has not taken", before: `{"n":2}`, status: coderwebsocket.StatusPolicyViolation},
		{name: "the frame ending the client's messages", after: "null", status: coderwebsocket.StatusNormalClosure},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answered := make(chan error, 1)
			release := make(chan struct{})
			url := serve(t, func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil, time.Second)
				if err != nil {
					answered <- err
					return
				}
				defer conn.Close(nil)
				if _, err = conn.Recv(); err != nil {
					answered <- err
					return
				}
				conn.CloseRecv()
				_, err = conn.Recv()
				answered <- err
				select {
				case <-conn.Context().Done():
				case <-release:
				}
			})

			client := dialLibrary(t, url)
			closed := make(chan error, 1)
			go func() {
				_, _, err := client.Read(context.WithoutCancel(t.Context()))
				closed <- err
			}()
			require.NoError(t, client.Write(t.Context(), coderwebsocket.MessageText, []byte(`{"n":1}`)))
			if tt.before != "" {
				require.NoError(t, client.Write(t.Context(), coderwebsocket.MessageText, []byte(tt.before)))
			}
			require.ErrorIs(t, await(t, answered), io.EOF, "Recv answers io.EOF once CloseRecv has been called")
			if tt.after != "" {
				require.NoError(t, client.Write(t.Context(), coderwebsocket.MessageText, []byte(tt.after)))
			}
			if tt.status == coderwebsocket.StatusNormalClosure {
				// The server has read what was sent once it answers a ping
				// sent after it, the connection still open.
				require.NoError(t, client.Ping(t.Context()))
				close(release)
			}
			require.Equal(t, tt.status, coderwebsocket.CloseStatus(await(t, closed)))
		})
	}
}

// TestCloseReturnsTheStatusTheConnectionClosedWith pins what Close returns:
// the status its error stands for, or the status the connection had closed
// with before, the one of the close a message its client may not send drew,
// the client's own when the client closed it, and StatusAbnormalClosure when
// the client left without closing.
func TestCloseReturnsTheStatusTheConnectionClosedWith(t *testing.T) {
	tests := []struct {
		name string
		// client is what the client does to end the connection before the
		// server closes it, nil for a client that waits for the close.
		client func(t *testing.T, client *coderwebsocket.Conn)
		ending error
		status int
	}{
		{name: "closed without an error", status: 1000},
		{name: "closed with a service error", ending: types.NewError(http.StatusNotFound, "sample not found"), status: 4404},
		{
			name: "closed before on what the client sent",
			client: func(t *testing.T, client *coderwebsocket.Conn) {
				t.Helper()
				require.NoError(t, client.Write(t.Context(), coderwebsocket.MessageBinary, []byte(`{"n":1}`)))
			},
			status: 1003,
		},
		{
			name: "closed by the client",
			client: func(t *testing.T, client *coderwebsocket.Conn) {
				t.Helper()
				_ = client.Close(4001, "sample")
			},
			ending: types.NewError(http.StatusNotFound, "sample not found"),
			status: 4001,
		},
		{
			name: "left by the client without a close",
			client: func(t *testing.T, client *coderwebsocket.Conn) {
				t.Helper()
				require.NoError(t, client.CloseNow())
			},
			status: 1006,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			returned := make(chan int, 1)
			url := serve(t, func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil, time.Second)
				if err != nil {
					returned <- -1
					return
				}
				if tt.client != nil {
					<-conn.Context().Done()
				}
				returned <- conn.Close(tt.ending)
			})

			client := dialLibrary(t, url)
			if tt.client != nil {
				go func() { _, _, _ = client.Read(context.WithoutCancel(t.Context())) }()
				tt.client(t, client)
			} else {
				_ = closedWith(t, client)
			}
			require.Equal(t, tt.status, await(t, returned))
		})
	}
}

// TestRecvAnswersTheErrorTheServerClosedWith proves the client end reads the
// ending back out of the status the server closed with: the service error
// with its status and message, the server's own failure, and the shutdown as
// the service being unavailable.
func TestRecvAnswersTheErrorTheServerClosedWith(t *testing.T) {
	tests := []struct {
		name   string
		ending error
		status int
		msg    string
	}{
		{name: "a service error", ending: types.NewError(http.StatusNotFound, "sample not found"), status: http.StatusNotFound, msg: "sample not found"},
		{name: "another error", ending: errors.New("sample failure"), status: http.StatusInternalServerError, msg: types.FailureMsg},
		{name: "the server shutting down", ending: streamctx.ErrServerShutdown, status: http.StatusServiceUnavailable, msg: "the server is shutting down"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := serve(t, func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil, time.Second)
				if err != nil {
					return
				}
				conn.Close(tt.ending)
			})

			_, err := recv(t, dial(t, url, nil))
			var ending *types.Error
			require.ErrorAs(t, err, &ending)
			require.Equal(t, tt.status, ending.Status())
			require.Equal(t, tt.msg, ending.Msg())
		})
	}
}

// TestRecvAnswersTheEndOfMessagesOnceTheClientHasLeft proves a client that
// has sent its last message is through, whatever ends the connection after
// it: the context having ended with the client leaving, Recv answers io.EOF
// every time, never the context's cause.
func TestRecvAnswersTheEndOfMessagesOnceTheClientHasLeft(t *testing.T) {
	answered := make(chan error, 1)
	url := serve(t, func(w http.ResponseWriter, r *http.Request) {
		answered <- func() error {
			conn, err := websocket.Accept(w, r, nil, time.Second)
			if err != nil {
				return err
			}
			defer conn.Close(nil)
			<-conn.Context().Done()
			// Both the end of the messages and the end of the context wait
			// for Recv, which must not pick between them by chance.
			for range 64 {
				if message, err := conn.Recv(); !errors.Is(err, io.EOF) {
					return errors.Newf("Recv answered %q and %v, not io.EOF", message, err)
				}
			}
			return nil
		}()
	})

	conn := dial(t, url, nil)
	require.NoError(t, conn.CloseSend())
	conn.Close(nil)
	require.NoError(t, await(t, answered))
}

// TestContextEndsWhenTheClientLeaves proves an accepted connection notices
// its client leaving while nothing reads from it: its context ends, canceled.
func TestContextEndsWhenTheClientLeaves(t *testing.T) {
	ended := make(chan error, 1)
	url := serve(t, func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil, time.Second)
		if err != nil {
			ended <- err
			return
		}
		defer conn.Close(nil)
		<-conn.Context().Done()
		ended <- context.Cause(conn.Context())
	})

	conn := dial(t, url, nil)
	conn.Close(nil)
	require.ErrorIs(t, await(t, ended), context.Canceled)
}

// TestContextEndsWhenTheServerShutsDown proves an accepted connection's
// context ends, with the shutdown as its cause, the moment the server the
// request arrived on begins to shut down: a send fails with that cause from
// then on, sending nothing, and closing the connection with it tells the
// client the server is going away.
func TestContextEndsWhenTheServerShutsDown(t *testing.T) {
	shutdown, beginShutdown := context.WithCancel(context.Background())
	ended := make(chan error, 1)
	url := serve(t, func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r.WithContext(streamctx.WithServerShutdown(r.Context(), shutdown)), nil, time.Second)
		if err != nil {
			ended <- err
			return
		}
		<-conn.Context().Done()
		ended <- conn.Send([]byte(`{"n":1}`))
		conn.Close(context.Cause(conn.Context()))
	})

	client := dialLibrary(t, url)
	beginShutdown()
	require.ErrorIs(t, await(t, ended), streamctx.ErrServerShutdown)
	require.Equal(t, coderwebsocket.StatusGoingAway, coderwebsocket.CloseStatus(closedWith(t, client)))
}

// TestSendFailsWhenTheClientStopsReading proves a client that stops reading
// holds a send for no longer than the write timeout: the send fails, and the
// connection's context ends.
func TestSendFailsWhenTheClientStopsReading(t *testing.T) {
	sent := make(chan error, 1)
	url := serve(t, func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil, 100*time.Millisecond)
		if err != nil {
			sent <- err
			return
		}
		defer conn.Close(nil)
		message := messageOfSize(1 << 20)
		for range 256 {
			if err := conn.Send(message); err != nil {
				<-conn.Context().Done()
				sent <- err
				return
			}
		}
		sent <- nil
	})

	dialLibrary(t, url)
	require.Error(t, await(t, sent), "a send to a client that reads nothing must fail")
}

// TestDrainWaitsForTheConnectionsAccepted proves Drain returns once every
// accepted connection has closed, and not before.
func TestDrainWaitsForTheConnectionsAccepted(t *testing.T) {
	release := make(chan struct{})
	url := serve(t, func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil, time.Second)
		if err != nil {
			return
		}
		<-release
		conn.Close(nil)
	})
	conn := dial(t, url, nil)

	drained := make(chan int, 1)
	go func() { drained <- websocket.Drain(t.Context()) }()
	select {
	case <-drained:
		t.Fatal("Drain must wait for the connection still open")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	require.Zero(t, await(t, drained), "no connection is left to cut")
	_, err := recv(t, conn)
	require.ErrorIs(t, err, io.EOF)
}

// TestDrainCutsTheConnectionsItsWindowLeftOpen proves Drain waits no longer
// than its context lasts: the connections still open then are cut, without
// the closing handshake, and counted.
func TestDrainCutsTheConnectionsItsWindowLeftOpen(t *testing.T) {
	release := make(chan struct{})
	url := serve(t, func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil, time.Second)
		if err != nil {
			return
		}
		defer conn.Close(nil)
		<-release
	})
	t.Cleanup(func() { close(release) })
	conn := dial(t, url, nil)

	window, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	require.Equal(t, 1, websocket.Drain(window))
	_, err := recv(t, conn)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF, "a connection cut is not one the server closed")
	require.NotErrorIs(t, err, io.EOF)
}

// serve starts a server handing every request to handle and returns its
// address. When the test ends it waits for every request to have been
// handled, so that no connection of one test is still open during the next,
// and fails the test when one is not in time: what a test registers to end
// its handlers, a client closing or a channel released, is registered after
// the call.
func serve(t *testing.T, handle http.HandlerFunc) string {
	t.Helper()

	var handling sync.WaitGroup
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handling.Add(1)
		defer handling.Done()
		handle(w, r)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() {
		handled := make(chan struct{})
		go func() {
			handling.Wait()
			close(handled)
		}()
		select {
		case <-handled:
		case <-time.After(5 * time.Second):
			t.Error("a request is still being handled")
		}
	})
	return srv.URL
}

// dial opens a connection to the server at url through Dial, sending header
// with the handshake, and closes it when the test ends.
func dial(t *testing.T, url string, header http.Header) *websocket.Conn {
	t.Helper()

	conn, _, err := websocket.Dial(t.Context(), nil, url, header, time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close(nil) })
	return conn
}

// dialLibrary opens a connection to the server at url through the library
// itself, for what a test sends or reads that Dial's connection does not: a
// binary message, the status a connection closed with.
func dialLibrary(t *testing.T, url string) *coderwebsocket.Conn {
	t.Helper()

	client, _, err := coderwebsocket.Dial(t.Context(), url, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.CloseNow() })
	return client
}

// closedWith returns the error the next read of client fails with, the close
// the server ended the connection with, and fails the test when the
// connection is still open after a moment.
func closedWith(t *testing.T, client *coderwebsocket.Conn) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, _, err := client.Read(ctx)
	require.NotErrorIs(t, err, context.DeadlineExceeded, "the server must have closed the connection")
	return err
}

// recv returns what conn's Recv answers, and fails the test when it answers
// nothing in time.
func recv(t *testing.T, conn *websocket.Conn) ([]byte, error) {
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
	answered := await(t, answers)
	return answered.message, answered.err
}

// messageOfSize returns a JSON message, a string, of exactly size bytes.
func messageOfSize(size int) []byte {
	return []byte(`"` + strings.Repeat("a", size-2) + `"`)
}

// await returns what results delivers, failing the test when nothing arrives
// in time.
func await[T any](t *testing.T, results <-chan T) T {
	t.Helper()

	select {
	case result := <-results:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("nothing arrived in time")
		panic("unreachable")
	}
}
