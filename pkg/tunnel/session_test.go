package tunnel_test

import (
	"fmt"
	"net"
	"testing"

	"github.com/hydroan/gst/bootstrap"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/pkg/tunnel"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	addr = "0.0.0.0:12345"

	Bye   = tunnel.NewCmd("bye", 1000)
	Hello = tunnel.NewCmd("hello", 1001)
)

type ByePaylod struct {
	Field1 string
	Field2 uint64
}
type HelloPaylod struct {
	Field3 string
	Field4 float64
}

var (
	byePayload1   = ByePaylod{Field1: "bye1", Field2: 123}
	byePayload2   = ByePaylod{Field1: "bye2", Field2: 456}
	helloPayload1 = HelloPaylod{Field3: "hello1", Field4: 3.14}
	helloPayload2 = HelloPaylod{Field3: "hello2", Field4: 3.14}
)

func TestSession(t *testing.T) {
	t.Setenv(config.DATABASE_AUTO_MIGRATE, "true")
	require.NoError(t, bootstrap.Bootstrap())
	readyCh := make(chan struct{}, 1)
	doneCh := make(chan struct{}, 1)
	errCh := make(chan error, 1)

	go server(readyCh, errCh)
	client(t, readyCh, doneCh)
	<-doneCh
	require.NoError(t, <-errCh)
}

func server(readyCh chan<- struct{}, errCh chan<- error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		readyCh <- struct{}{}
		errCh <- err
		return
	}
	defer l.Close()

	readyCh <- struct{}{}

	conn, err := l.Accept()
	if err != nil {
		errCh <- err
		return
	}
	defer conn.Close()

	session, _ := tunnel.NewSession(conn, consts.Server)
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
			payload := new(HelloPaylod)
			if err := tunnel.DecodePayload(event.Payload, payload); err != nil {
				errCh <- err
				return
			}
			if helloPayload1 != *payload {
				errCh <- fmt.Errorf("expected hello payload %+v, got %+v", helloPayload1, *payload)
				return
			}
			if err := session.Write(&tunnel.Event{Cmd: Hello, Payload: helloPayload2}); err != nil {
				errCh <- err
				return
			}
		case Bye:
			payload := new(ByePaylod)
			if err := tunnel.DecodePayload(event.Payload, payload); err != nil {
				errCh <- err
				return
			}
			if byePayload1 != *payload {
				errCh <- fmt.Errorf("expected bye payload %+v, got %+v", byePayload1, *payload)
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

func client(t *testing.T, readyCh <-chan struct{}, doneCh chan<- struct{}) {
	t.Helper()
	<-readyCh

	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer conn.Close()

	session, _ := tunnel.NewSession(conn, consts.Client)
	_ = session.Write(&tunnel.Event{Cmd: tunnel.Ping})

	for {
		event, err := session.Read()
		require.NoError(t, err)

		switch event.Cmd {
		case tunnel.Pong:
			t.Log("server pong")
			_ = session.Write(&tunnel.Event{Cmd: Hello, Payload: helloPayload1})
		case Hello:
			payload := new(HelloPaylod)
			require.NoError(t, tunnel.DecodePayload(event.Payload, payload))
			assert.Equal(t, helloPayload2, *payload)
			t.Logf("server hello: %+v\n", *payload)
			_ = session.Write(&tunnel.Event{Cmd: Bye, Payload: byePayload1})
		case Bye:
			payload := new(ByePaylod)
			require.NoError(t, tunnel.DecodePayload(event.Payload, payload))
			assert.Equal(t, byePayload2, *payload)
			t.Logf("server bye: %+v\n", *payload)
			doneCh <- struct{}{}
			return
		}
	}
}
