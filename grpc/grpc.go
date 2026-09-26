// Package grpc is the gRPC listener's API for a project's own code. The
// generated pb package registers the services of the models declaring
// GRPC() with Register and runs their actions through the call functions,
// CreateCall and its kind for the standard actions and ServiceCall for the
// actions with a payload or result of their own (see call.go); an
// interceptor of the project or of a copied module reads and establishes
// what a call carries — its caller, the HTTP action it maps to (see
// context.go) — and answers a failure with the status it maps to (see
// status.go). The listener itself and its chain of interceptors are the
// framework's own; the interceptors a project mounts are registered through
// package interceptor, the counterpart of package middleware.
package grpc

import (
	"github.com/hydroan/gst/internal/grpcserver"
	"google.golang.org/grpc"
)

// Method describes one rpc of a registered service the way the generated
// registration file declares it: its full name, whether its action declares
// Public(), and the HTTP method and route the same action is served at over
// HTTP, which Route answers and the interceptors of the modules judge a call
// by.
type Method = grpcserver.Method

// Register queues fn to register a service on the listener, the way the
// generated pb/pb.gen.go registers the service of every model declaring
// GRPC(): fn gets the server and calls the RegisterXxxServiceServer function
// the protobuf plugin generated, and methods describe the service's rpcs.
// It runs at package initialization, before bootstrap starts the listeners,
// and panics once the listener runs.
func Register(fn func(grpc.ServiceRegistrar), methods ...Method) {
	grpcserver.Register(fn, methods...)
}
