package grpcserver

import (
	"context"
	"sync"

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
// stream when the HTTP listener does, ends a read waiting on the client
// with it (see shutdownStream), and answers a stream the stop ended with
// Unavailable, the status the gRPC conventions give a server shutting
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
		err := handler(srv, &shutdownStream{ServerStream: ss, ctx: ctx})
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

// shutdownStream is the stream the handler gets from streamShutdown: the
// stream as the chain handed it, on ctx, the context the stop ends, with
// reads that end with ctx. grpc-go's RecvMsg returns for a message, for
// the client ending the stream, or for the stream's own context ending,
// which the server ends only once the handler returned: a handler waiting
// on the client for its next message, a service in Recv or the generated
// handler reading the first message for the route parameters, would wait
// past the stop to the end of its window, and be cut there. So the stream
// reads on a goroutine of its own, one for as long as the handler reads,
// and RecvMsg waits for whichever comes first, the read or ctx ending, the
// way go-control-plane's xDS server reads its stream through a channel it
// selects on beside its context. From ctx's end on, RecvMsg answers the
// status of ctx, Canceled, without reading, which streamShutdown turns
// into Unavailable for a stop; a read ctx overtook completes on the
// goroutine once the handler returned and grpc-go canceled the stream, its
// message dropped. Until then the goroutine may still write the message
// object that read was handed, which a caller must not read after an
// error: the generated Recv allocates one per call and drops it on an
// error, the way grpc-go's own RecvMsg leaves it in an unspecified state.
type shutdownStream struct {
	grpc.ServerStream
	ctx     context.Context
	reading sync.Once  // starts the goroutine, on the first read
	wants   chan any   // the message the next read decodes into
	reads   chan error // what the read answered
}

func (s *shutdownStream) Context() context.Context { return s.ctx }

func (s *shutdownStream) RecvMsg(m any) error {
	s.reading.Do(func() {
		s.wants, s.reads = make(chan any), make(chan error)
		go s.read()
	})
	if s.ctx.Err() != nil {
		return s.ended()
	}
	select {
	case s.wants <- m:
	case <-s.ctx.Done():
		return s.ended()
	}
	select {
	case err := <-s.reads:
		return err
	case <-s.ctx.Done():
		return s.ended()
	}
}

// read serves the reads of RecvMsg, one at a time, until ctx ends.
func (s *shutdownStream) read() {
	for {
		select {
		case m := <-s.wants:
			err := s.ServerStream.RecvMsg(m)
			select {
			case s.reads <- err:
			case <-s.ctx.Done():
				return
			}
		case <-s.ctx.Done():
			return
		}
	}
}

// ended is the status a read ctx ended answers: Canceled, or
// DeadlineExceeded past a deadline the client set.
func (s *shutdownStream) ended() error {
	return status.FromContextError(s.ctx.Err()).Err()
}
