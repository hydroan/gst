// Package interceptor is the gRPC counterpart of package middleware: the
// interceptors a project mounts on the gRPC listener. Register adds
// interceptors to every method, RegisterAuth to the methods not declared
// Public(), and the constructors here build the interceptors the framework
// ships for either; IAMSession and Authz are the interceptors of the iam and
// authz modules, whose source files gg module copy copies into a project
// serving gRPC. What an interceptor reads and establishes about a call, its
// caller and the HTTP action it maps to, is package grpc's.
//
// The chain the framework runs on every call itself — request scope,
// metrics, recovery — and the machinery that mounts registered interceptors
// behind it are the framework's own, in the server package.
package interceptor

import (
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
