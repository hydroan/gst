package grpcserver

import (
	"context"
	"testing"
	"time"

	"github.com/hydroan/gst/consts"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// TestAPanicAnswersInternalAndIsLoggedWithItsStack pins the bounds of a
// panic in a handler: the caller gets codes.Internal with the message the
// HTTP envelope carries for the same case, the server goes on answering,
// and the recovery log, the one the HTTP listener's panics go to, has the
// panic, the method, the trace id and the stack.
func TestAPanicAnswersInternalAndIsLoggedWithItsStack(t *testing.T) {
	reset(t)
	serve(map[string]func(context.Context) error{
		"Boom": func(context.Context) error { panic("boom") },
		"Ping": func(context.Context) error { return nil },
	})
	conn := dial(t, start(t), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "x-trace-id", "trace-panic")
	var header metadata.MD

	err := call(ctx, conn, "Boom", grpc.Header(&header))

	require.Equal(t, codes.Internal, status.Code(err))
	require.Equal(t, "internal server error", status.Convert(err).Message())
	require.Equal(t, []string{"trace-panic"}, header.Get("x-trace-id"), "the caller quotes the trace id back from the header of the failed call")
	require.NoError(t, call(ctx, conn, "Ping"), "the server must outlive the panic")
	entries := recoveryLog.All()
	require.Len(t, entries, 1)
	require.Contains(t, entries[0].Message, "panic recovered")
	require.Contains(t, entries[0].Message, "boom")
	require.Contains(t, entries[0].Message, "/gst.test.Echo/Boom")
	require.Contains(t, entries[0].Message, "goroutine ", "the stack of the panic")
	require.Equal(t, "trace-panic", entries[0].ContextMap()[consts.TRACE_ID])
}
