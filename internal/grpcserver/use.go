package grpcserver

import (
	"context"

	grpcmiddleware "github.com/grpc-ecosystem/go-grpc-middleware/v2"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/selector"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/logfield"
	"github.com/hydroan/gst/logger"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// Interceptor is an interceptor a project or a copied module mounts through
// Use or UseAuth: it reads what the call carries, its metadata, its caller,
// the action it maps to (see Route), and returns the context the call goes
// on with, the caller established on it (see WithCaller), or the error
// refusing the call: a status error answers as it is, a service error with
// the code its HTTP status maps to and its message, any other error as
// Internal (see refused). The one form serves a unary call and a stream
// alike, which is why an interceptor here transforms the context rather
// than wrapping the handler the way a grpc.UnaryServerInterceptor does: an
// authentication runs once, ahead of the first message either way. The
// public grpc.Interceptor forwards to it.
type Interceptor func(ctx context.Context) (context.Context, error)

// commonInterceptors and authInterceptors are the interceptors Use and
// UseAuth queued, in order: the project's, run inside the framework's own
// chain on every method and on the non-public methods respectively.
var (
	commonInterceptors []Interceptor
	authInterceptors   []Interceptor
)

// Use queues interceptors to run on every call, unary or stream, after the
// framework's own chain and before the ones UseAuth queued, in the order
// given, the way the HTTP listener runs the middleware Register adds on
// every route; the calls of the server's own services are left alone (see
// ownServices), the way the listener's probes run outside that middleware.
// The public interceptor.Register forwards to it. Like Register it runs at
// package initialization; queuing once the server runs would intercept
// nothing, so it panics.
func Use(interceptors ...Interceptor) {
	mu.Lock()
	defer mu.Unlock()
	if started.Load() {
		panic("grpcserver: Use after the server started; register interceptors at package initialization")
	}
	commonInterceptors = append(commonInterceptors, interceptors...)
}

// UseAuth queues interceptors to run on every call to a method not declared
// public, after the ones Use queued, in the order given: the place for
// authentication and authorization, the way the HTTP listener runs the
// middleware RegisterAuth adds on the routes of its authenticated group. The
// public interceptor.RegisterAuth forwards to it. Like Use it panics once
// the server runs.
func UseAuth(interceptors ...Interceptor) {
	mu.Lock()
	defer mu.Unlock()
	if started.Load() {
		panic("grpcserver: UseAuth after the server started; register interceptors at package initialization")
	}
	authInterceptors = append(authInterceptors, interceptors...)
}

// projectCall matches the calls the common interceptors run on: every call
// but those of the server's own services.
var projectCall = selector.MatchFunc(func(_ context.Context, meta interceptors.CallMeta) bool {
	return !ownServices[meta.Service]
})

// guarded matches the calls the auth interceptors run on (see requiresAuth).
var guarded = selector.MatchFunc(func(_ context.Context, meta interceptors.CallMeta) bool {
	return requiresAuth(meta.Service, meta.FullMethod())
})

// projectUnaryInterceptors returns the interceptors Use and UseAuth queued
// as the unary chain runs them, in order: the common ones behind the
// selector leaving the server's own services alone, then the auth ones
// behind the one leaving the public methods alone as well.
func projectUnaryInterceptors() []grpc.UnaryServerInterceptor {
	var chain []grpc.UnaryServerInterceptor
	for _, ic := range commonInterceptors {
		chain = append(chain, selector.UnaryServerInterceptor(unaryOf(ic), projectCall))
	}
	for _, ic := range authInterceptors {
		chain = append(chain, selector.UnaryServerInterceptor(unaryOf(ic), guarded))
	}
	return chain
}

// projectStreamInterceptors returns the same interceptors as the stream
// chain runs them, in the same order and behind the same selectors.
func projectStreamInterceptors() []grpc.StreamServerInterceptor {
	var chain []grpc.StreamServerInterceptor
	for _, ic := range commonInterceptors {
		chain = append(chain, selector.StreamServerInterceptor(streamOf(ic), projectCall))
	}
	for _, ic := range authInterceptors {
		chain = append(chain, selector.StreamServerInterceptor(streamOf(ic), guarded))
	}
	return chain
}

// unaryOf runs ic on a unary call: the handler gets the context ic
// returns, or the call is refused with the status its error maps to (see
// refused).
func unaryOf(ic Interceptor) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		next, err := ic(ctx)
		if err != nil {
			return nil, refused(ctx, err)
		}
		return handler(next, req)
	}
}

// streamOf runs ic on a stream, once, ahead of the first message: the
// handler gets the stream on the context ic returns, or the stream is
// refused with the status its error maps to (see refused).
func streamOf(ic Interceptor) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		next, err := ic(ss.Context())
		if err != nil {
			return refused(ss.Context(), err)
		}
		return handler(srv, withStreamContext(next, ss))
	}
}

// refused maps the error an interceptor returned to the status the call is
// refused with: a status error as it is; a service error, gst.Error, by the
// code its HTTP status maps to and its client-safe message, what the HTTP
// middleware's Abort answers for the same refusal; any other error as
// Internal with the server failure message (see StatusError). gRPC itself
// would answer any other error as Unknown with its text, the wrapped cause
// included. The error goes to the gRPC log first, cause included, since the
// answer leaves that out, the way a failing service operation is logged
// before it is mapped.
func refused(ctx context.Context, err error) error {
	if _, ok := status.FromError(err); ok {
		return err
	}
	if logger.GRPC != nil {
		logger.GRPC.Error("interceptor refused the call", zap.Error(err), logfield.TraceID(execctx.FromContext(ctx).TraceID))
	}
	return StatusError(err)
}

// withStreamContext returns ss carrying ctx as its context, what the
// handler and the calls downstream read the call's metadata and caller
// from.
func withStreamContext(ctx context.Context, ss grpc.ServerStream) grpc.ServerStream {
	wrapped := grpcmiddleware.WrapServerStream(ss)
	wrapped.WrappedContext = ctx
	return wrapped
}
