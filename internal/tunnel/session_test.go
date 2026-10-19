package tunnel_test

import (
	"encoding/binary"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/bootstrap"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/testutil/testlog"
	"github.com/hydroan/gst/internal/tunnel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	Bye   = tunnel.NewCmd("bye", 1000)
	Hello = tunnel.NewCmd("hello", 1001)
)

type ByePayload struct {
	Field1 string
	Field2 uint64
}
type HelloPayload struct {
	Field3 string
	Field4 float64
}

var (
	byePayload1   = ByePayload{Field1: "bye1", Field2: 123}
	byePayload2   = ByePayload{Field1: "bye2", Field2: 456}
	helloPayload1 = HelloPayload{Field3: "hello1", Field4: 3.14}
	helloPayload2 = HelloPayload{Field3: "hello2", Field4: 3.14}
)

// TestMain gives the process TestSession bootstraps a scratch log directory,
// which keeps its logs out of the test output, and removes it once the tests
// are done.
func TestMain(m *testing.M) {
	_, releaseLogs, err := testlog.ToTempDir()
	if err != nil {
		panic(err)
	}

	code := m.Run()
	_ = releaseLogs()
	os.Exit(code)
}

func TestSession(t *testing.T) {
	t.Setenv(config.DATABASE_AUTO_MIGRATE, "true")
	require.NoError(t, bootstrap.Bootstrap())
	// The listener picks its own port and hands the address over, so parallel
	// test binaries never contend for one, and the client cannot dial before
	// the listener exists.
	addrCh := make(chan string, 1)
	doneCh := make(chan struct{}, 1)
	errCh := make(chan error, 1)

	go server(addrCh, errCh)
	client(t, addrCh, doneCh)
	<-doneCh
	require.NoError(t, <-errCh)
}

// TestReadRefusesAnOversizeFrame pins the frame limit on the read side: a
// header announcing more than 4 MiB is refused before its body is read, and
// the connection is closed, since the stream past the refused header holds
// no header to read next.
func TestReadRefusesAnOversizeFrame(t *testing.T) {
	serverConn, clientConn := tcpPair(t)
	session, err := tunnel.NewSession(serverConn, tunnel.Server)
	require.NoError(t, err)

	const overLimit = 4<<20 + 1 // one byte over the 4 MiB frame limit
	require.NoError(t, binary.Write(clientConn, binary.BigEndian, uint32(overLimit)))
	require.NoError(t, serverConn.SetReadDeadline(time.Now().Add(5*time.Second)))

	_, err = session.Read()
	require.Error(t, err)
	requireClosedByPeer(t, clientConn)
}

// TestWriteRefusesAnOversizeEvent pins the frame limit on the write side: an
// event that encodes to more than 4 MiB is refused and nothing of it reaches
// the peer.
func TestWriteRefusesAnOversizeEvent(t *testing.T) {
	serverConn, clientConn := tcpPair(t)
	session, err := tunnel.NewSession(clientConn, tunnel.Client)
	require.NoError(t, err)
	require.NoError(t, clientConn.SetWriteDeadline(time.Now().Add(5*time.Second)))

	err = session.Write(&tunnel.Event{Cmd: Hello, Payload: strings.Repeat("x", 4<<20)})
	require.Error(t, err)

	require.NoError(t, serverConn.SetReadDeadline(time.Now().Add(time.Second)))
	n, err := serverConn.Read(make([]byte, 1))
	require.Zero(t, n)
	require.ErrorIs(t, err, os.ErrDeadlineExceeded)
}

// TestSessionsFromOneHostAreListedApart pins the key of a connection without
// a client ID: its local and remote addresses, so two connections from one
// host are two entries, and so are the two ends of one connection.
func TestSessionsFromOneHostAreListedApart(t *testing.T) {
	firstServer, firstClient := tcpPair(t)
	secondServer, _ := tcpPair(t)

	_, err := tunnel.NewSession(firstServer, tunnel.Server)
	require.NoError(t, err)
	_, err = tunnel.NewSession(secondServer, tunnel.Server)
	require.NoError(t, err)
	_, err = tunnel.NewSession(firstClient, tunnel.Client)
	require.NoError(t, err)

	listed := tunnel.List()
	for _, conn := range []net.Conn{firstServer, secondServer, firstClient} {
		require.Contains(t, listed, conn.LocalAddr().String()+"->"+conn.RemoteAddr().String())
	}
}

// TestCloseRemovesTheSessionFromTheList pins Close: it closes the
// connection and takes the session out of the registration List reads.
func TestCloseRemovesTheSessionFromTheList(t *testing.T) {
	serverConn, clientConn := tcpPair(t)
	session, err := tunnel.NewSession(serverConn, tunnel.Server)
	require.NoError(t, err)
	key := serverConn.LocalAddr().String() + "->" + serverConn.RemoteAddr().String()
	require.Contains(t, tunnel.List(), key)

	require.NoError(t, session.Close())
	require.NotContains(t, tunnel.List(), key)
	requireClosedByPeer(t, clientConn)
}

// TestAClientIDThatConnectsAgainTakesItsKeyOver pins how the server side
// treats a client ID that is already connected: the newer connection takes
// the key over and the older one is closed, the way MQTT treats a client ID
// that connects again; the client often reconnects before the server has
// noticed its older connection is gone.
func TestAClientIDThatConnectsAgainTakesItsKeyOver(t *testing.T) {
	const id = "sample-client"
	olderServer, olderClient := tcpPair(t)
	newerServer, newerClient := tcpPair(t)

	older, err := tunnel.NewSession(olderServer, tunnel.Server, id)
	require.NoError(t, err)
	newer, err := tunnel.NewSession(newerServer, tunnel.Server, id)
	require.NoError(t, err)
	requireClosedByPeer(t, olderClient)

	// Closing the older session leaves the key to the newer one.
	require.ErrorIs(t, older.Close(), net.ErrClosed)
	require.Contains(t, tunnel.List(), id)

	peer, err := tunnel.NewSession(newerClient, tunnel.Client)
	require.NoError(t, err)
	require.NoError(t, newer.Write(&tunnel.Event{Cmd: tunnel.Ping}))
	event, err := peer.Read()
	require.NoError(t, err)
	require.Equal(t, tunnel.Ping, event.Cmd)
}

// TestNewSessionRefusesAnUnknownSide pins that a side other than Server and
// Client is an error the caller gets back.
func TestNewSessionRefusesAnUnknownSide(t *testing.T) {
	serverConn, _ := tcpPair(t)

	session, err := tunnel.NewSession(serverConn, tunnel.AppSide("peer"))
	require.Error(t, err)
	require.Nil(t, session)
}

// TestConcurrentReadsGetWholeEvents pins that a frame is read in one step:
// two goroutines reading one session each get whole events, never the
// header of one frame with the body of another.
func TestConcurrentReadsGetWholeEvents(t *testing.T) {
	serverConn, clientConn := tcpPair(t)
	reader, err := tunnel.NewSession(serverConn, tunnel.Server)
	require.NoError(t, err)
	writer, err := tunnel.NewSession(clientConn, tunnel.Client)
	require.NoError(t, err)

	const events = 500
	go func() {
		for range events {
			if writer.Write(&tunnel.Event{Cmd: Hello, Payload: helloPayload1}) != nil {
				return
			}
		}
		_ = clientConn.(*net.TCPConn).CloseWrite() //nolint:errcheck
	}()

	var (
		mu   sync.Mutex
		got  int
		errs []error
		wg   sync.WaitGroup
	)
	for range 2 {
		wg.Go(func() {
			for {
				event, err := reader.Read()
				mu.Lock()
				switch {
				case errors.Is(err, io.EOF):
				case err != nil:
					errs = append(errs, err)
				case event.Cmd == Hello:
					got++
				default:
					errs = append(errs, errors.Newf("unexpected event %v", event.Cmd))
				}
				mu.Unlock()
				if err != nil {
					return
				}
			}
		})
	}
	wg.Wait()
	require.Empty(t, errs)
	require.Equal(t, events, got)
}

func server(addrCh chan<- string, errCh chan<- error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		addrCh <- ""
		errCh <- err
		return
	}
	defer l.Close()

	addrCh <- l.Addr().String()

	conn, err := l.Accept()
	if err != nil {
		errCh <- err
		return
	}
	defer conn.Close()

	session, _ := tunnel.NewSession(conn, tunnel.Server)
	for {
		event, err := session.Read()
		if err != nil {
			errCh <- err
			return
		}
		switch event.Cmd {
		case tunnel.Ping:
			if err := session.Write(&tunnel.Event{Cmd: tunnel.Pong}); err != nil {
				errCh <- err
				return
			}
		case Hello:
			payload := new(HelloPayload)
			if err := tunnel.DecodePayload(event.Payload, payload); err != nil {
				errCh <- err
				return
			}
			if helloPayload1 != *payload {
				errCh <- errors.Newf("expected hello payload %+v, got %+v", helloPayload1, *payload)
				return
			}
			if err := session.Write(&tunnel.Event{Cmd: Hello, Payload: helloPayload2}); err != nil {
				errCh <- err
				return
			}
		case Bye:
			payload := new(ByePayload)
			if err := tunnel.DecodePayload(event.Payload, payload); err != nil {
				errCh <- err
				return
			}
			if byePayload1 != *payload {
				errCh <- errors.Newf("expected bye payload %+v, got %+v", byePayload1, *payload)
				return
			}
			if err := session.Write(&tunnel.Event{Cmd: Bye, Payload: byePayload2}); err != nil {
				errCh <- err
				return
			}
			errCh <- nil
			return
		}
	}
}

func client(t *testing.T, addrCh <-chan string, doneCh chan<- struct{}) {
	t.Helper()
	addr := <-addrCh
	require.NotEmpty(t, addr, "listener failed to start")

	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer conn.Close()

	session, _ := tunnel.NewSession(conn, tunnel.Client)
	_ = session.Write(&tunnel.Event{Cmd: tunnel.Ping})

	for {
		event, err := session.Read()
		require.NoError(t, err)

		switch event.Cmd {
		case tunnel.Pong:
			t.Log("server pong")
			_ = session.Write(&tunnel.Event{Cmd: Hello, Payload: helloPayload1})
		case Hello:
			payload := new(HelloPayload)
			require.NoError(t, tunnel.DecodePayload(event.Payload, payload))
			assert.Equal(t, helloPayload2, *payload)
			t.Logf("server hello: %+v\n", *payload)
			_ = session.Write(&tunnel.Event{Cmd: Bye, Payload: byePayload1})
		case Bye:
			payload := new(ByePayload)
			require.NoError(t, tunnel.DecodePayload(event.Payload, payload))
			assert.Equal(t, byePayload2, *payload)
			t.Logf("server bye: %+v\n", *payload)
			doneCh <- struct{}{}
			return
		}
	}
}

// tcpPair returns both ends of a TCP connection over the loopback, closed
// when the test ends.
func tcpPair(t *testing.T) (server, client net.Conn) {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	client, err = net.Dial("tcp", l.Addr().String())
	require.NoError(t, err)
	server, err = l.Accept()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = server.Close()
		_ = client.Close()
	})
	return server, client
}

// requireClosedByPeer fails the test unless the other end of conn closes
// the connection within five seconds.
func requireClosedByPeer(t *testing.T, conn net.Conn) {
	t.Helper()

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, err := conn.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF)
}
