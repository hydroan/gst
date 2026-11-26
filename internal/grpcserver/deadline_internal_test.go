package grpcserver

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/gst"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// TestAUnaryCallOutOfTimeIsAnsweredDeadlineExceeded pins what the server
// answers a unary call it could not answer within server.write_timeout, the
// time the HTTP listener gives a request as well: DeadlineExceeded with
// gst.TimeoutMsg, the message of the HTTP listener's 504, whether the
// handler stopped when its context ended or ignored the context and answered
// late. A call answered in time keeps its answer.
func TestAUnaryCallOutOfTimeIsAnsweredDeadlineExceeded(t *testing.T) {
	reset(t)
	config.App.Server.WriteTimeout = 50 * time.Millisecond
	serve(map[string]func(context.Context) error{
		"Prompt": func(context.Context) error { return nil },
		"Stopped": func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
		"Late": func(context.Context) error {
			time.Sleep(200 * time.Millisecond)
			return nil
		},
	})
	conn := dial(t, start(t), nil)

	require.NoError(t, call(context.Background(), conn, "Prompt"))
	for _, name := range []string{"Stopped", "Late"} {
		err := call(context.Background(), conn, name)
		require.Equal(t, codes.DeadlineExceeded, status.Code(err), name)
		require.Equal(t, gst.TimeoutMsg, status.Convert(err).Message(), name)
	}
}

// TestTheDeadlineOfACallIsTheSoonerOfTheClientsAndTheServers pins whose
// deadline a call runs under: the server's, server.write_timeout, for a call
// its client gave no deadline, and the client's when that one comes sooner.
// A stream runs under the client's alone: the server gives it none.
func TestTheDeadlineOfACallIsTheSoonerOfTheClientsAndTheServers(t *testing.T) {
	reset(t)
	config.App.Server.WriteTimeout = time.Minute
	left := make(chan time.Duration, 1)
	bounded := make(chan bool, 1)
	serveWith(map[string]func(context.Context) error{"Ping": func(ctx context.Context) error {
		ends, _ := ctx.Deadline()
		left <- time.Until(ends)
		return nil
	}}, map[string]grpc.StreamDesc{"Watch": {ServerStreams: true, Handler: func(_ any, ss grpc.ServerStream) error {
		_, ok := ss.Context().Deadline()
		bounded <- ok
		return nil
	}}})
	conn := dial(t, start(t), nil)

	require.NoError(t, call(context.Background(), conn, "Ping"))
	servers := <-left
	require.Greater(t, servers, 30*time.Second)
	require.LessOrEqual(t, servers, time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	require.NoError(t, call(ctx, conn, "Ping"))
	require.LessOrEqual(t, <-left, 20*time.Second)

	watching, err := conn.NewStream(context.Background(), &grpc.StreamDesc{StreamName: "Watch", ServerStreams: true}, "/gst.test.Echo/Watch")
	require.NoError(t, err)
	require.False(t, <-bounded, "the server gives a stream no deadline")
	require.ErrorIs(t, watching.RecvMsg(&emptypb.Empty{}), io.EOF)
}
