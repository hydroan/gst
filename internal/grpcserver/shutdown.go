package grpcserver

import (
	"context"

	"github.com/cockroachdb/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// errServerShutdown is the cause the context of a stream ends with when the
// server begins to stop, as opposed to the client going away: the
// counterpart of sse.ErrServerShutdown for the HTTP listener's streams.
var errServerShutdown = errors.New("grpc: the server is shutting down")

// shutdownMsg is the status message a stream the stop ended is answered
// with.
const shutdownMsg = "the server is shutting down"

// streamShutdown returns the stream interceptor closest to the handler: it
// gives the stream a context that ends the moment the server begins to
// stop (see Stop), the way sse.StreamContext ends a Server-Sent Events
// stream when the HTTP listener does, and answers a stream the stop ended
// with Unavailable, the status the gRPC conventions give a server shutting
// down, so that the client resumes on another replica. GracefulStop alone
// tells a stream nothing: it waits for every handler, and a stream serving
// a client who stays would hold the shutdown to the end of its window,
// then be cut. A unary call is left alone, to run to completion within the
// window. The stop ended a stream when its context ended with the shutdown
// and the handler then returned nil, the stream over, or Canceled, the
// context's status; every stage outside, the metrics, the access log and
// the tracing, records the stream as Unavailable. A client stream that
// answered before its context ended keeps its answer: a response already
// built is not thrown away for a retry that would repeat its writes.
func streamShutdown(stopping context.Context) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx, cancel := context.WithCancelCause(ss.Context())
		defer cancel(nil)
		stopWatch := context.AfterFunc(stopping, func() { cancel(errServerShutdown) })
		defer stopWatch()
		err := handler(srv, withStreamContext(ctx, ss))
		switch {
		case !errors.Is(context.Cause(ctx), errServerShutdown):
			return err
		case err == nil && !info.IsServerStream:
			return nil
		case err == nil || status.Code(err) == codes.Canceled:
			return status.Error(codes.Unavailable, shutdownMsg)
		}
		return err
	}
}
