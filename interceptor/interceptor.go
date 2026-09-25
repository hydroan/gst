// Package interceptor is the gRPC counterpart of package middleware: the
// interceptors a project mounts on the gRPC listener. Register adds
// interceptors to every method, RegisterAuth to the methods not declared
// Public(), and the constructors here build the interceptors the framework
// ships for either; the interceptors of the modules gg module copy copies
// into a project come with them, under this directory.
//
// The chain the framework runs on every call itself — request scope,
// metrics, recovery — and the machinery that mounts registered interceptors
// behind it are the framework's own, in the server package.
package interceptor

import (
	"context"

	"github.com/hydroan/gst/internal/grpcserver"
	"google.golang.org/grpc"
)

// Register adds interceptors that run on every call, in registration order,
// after the framework's own chain and ahead of every interceptor RegisterAuth
// adds. Call it from an init function: the server takes the interceptors
// registered when it starts.
func Register(interceptors ...grpc.UnaryServerInterceptor) {
	grpcserver.Use(interceptors...)
}

// RegisterAuth adds interceptors that run only on the calls to methods not
// declared Public(): the place for authentication and authorization. Call
// it from an init function: the server takes the interceptors registered
// when it starts. They run in registration order, after every interceptor
// Register adds.
func RegisterAuth(interceptors ...grpc.UnaryServerInterceptor) {
	grpcserver.UseAuth(interceptors...)
}

// Identity is who a call is made by, as an authentication interceptor
// established it: the fields the HTTP authentication middleware sets on the
// gin context.
type Identity = grpcserver.Identity

// WithIdentity returns ctx with identity established as the caller of the
// call: the request metadata the service context and the flows read names
// the caller, and so does the call's access-log entry. An authentication
// interceptor calls it once it has verified who is calling, and hands the
// returned context on to the handler; a gRPC context cannot be written the
// way a gin context can, which is why the identity travels this way.
func WithIdentity(ctx context.Context, identity Identity) context.Context {
	return grpcserver.WithIdentity(ctx, identity)
}
