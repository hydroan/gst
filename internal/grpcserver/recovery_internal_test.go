package grpcserver

import (
	"context"
	"testing"
	"time"

	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/i18n"
	"github.com/hydroan/gst/internal/types"
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
	const traceID = "0f0e0d0c0b0a09080706050403020100"
	ctx = metadata.AppendToOutgoingContext(ctx, "x-trace-id", traceID)
	var header metadata.MD

	err := call(ctx, conn, "Boom", grpc.Header(&header))

	require.Equal(t, codes.Internal, status.Code(err))
	require.Equal(t, types.FailureMsg, status.Convert(err).Message())
	require.Equal(t, []string{traceID}, header.Get("x-trace-id"), "the caller quotes the trace id back from the header of the failed call")
	require.NoError(t, call(ctx, conn, "Ping"), "the server must outlive the panic")
	entries := recoveryLog.All()
	require.Len(t, entries, 1)
	require.Contains(t, entries[0].Message, "panic recovered")
	require.Contains(t, entries[0].Message, "boom")
	require.Contains(t, entries[0].Message, "/gst.test.Echo/Boom")
	require.Contains(t, entries[0].Message, "goroutine ", "the stack of the panic")
	require.Equal(t, traceID, entries[0].ContextMap()[execctx.KeyTraceID])
}

// TestAPanicAndTheStopAnswerInTheLanguageOfTheProcess pins the language of
// the two answers the server words itself rather than through StatusError: a
// panic is answered Internal, and a stream the stop ended Unavailable, each
// with its sentence in the language the process answers in, the one the HTTP
// listener answers the same case in.
func TestAPanicAndTheStopAnswerInTheLanguageOfTheProcess(t *testing.T) {
	require.NoError(t, i18n.SetLanguage(i18n.Chinese))
	t.Cleanup(func() { require.NoError(t, i18n.SetLanguage(i18n.English)) })

	panicked := recovered(context.Background(), "boom")
	require.Equal(t, codes.Internal, status.Code(panicked))
	require.Equal(t, "服务器无法处理该请求。", status.Convert(panicked).Message())

	stopping, stop := context.WithCancel(context.Background())
	stop()
	// The handler returns once the stop has ended the context of its stream,
	// the way a stream waiting on its client does.
	stopped := streamShutdown(stopping)(nil, contextStream{ctx: context.Background()}, &grpc.StreamServerInfo{IsServerStream: true},
		func(_ any, ss grpc.ServerStream) error {
			<-ss.Context().Done()
			return nil
		})
	require.Equal(t, codes.Unavailable, status.Code(stopped))
	require.Equal(t, "服务器正在关闭", status.Convert(stopped).Message())
}

// contextStream is a server stream that only answers its context.
type contextStream struct {
	grpc.ServerStream

	ctx context.Context
}

// Context returns the context the stream was given.
func (s contextStream) Context() context.Context { return s.ctx }
