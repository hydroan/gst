// Package grpc is the gRPC listener's API for a project's own code. The
// generated pb package registers the services of the models declaring
// GRPC() with Register and runs their actions through the call functions,
// CreateCall and its kind for the standard actions, ServiceCall for the
// actions with a payload or result of their own (see call.go) and
// ServerStreamCall and its kind for the Stream actions (see stream.go); an
// interceptor of the project or of a copied module (see Interceptor) reads
// and establishes what a call carries — its caller, the HTTP action it maps
// to (see context.go) — and answers a failure with the status it maps to
// (see status.go). The listener itself and its chain of interceptors are
// the framework's own; the interceptors a project mounts are registered
// through package interceptor, the counterpart of package middleware.
package grpc

import (
	"github.com/hydroan/gst/internal/grpcserver"
	"google.golang.org/grpc"
)

// Interceptor is an interceptor a project or a copied module mounts through
// interceptor.Register or interceptor.RegisterAuth: it reads what the call
// carries, its metadata, its caller (see CallerOf) and the action it maps
// to (see Route), and returns the context the call goes on with, the
// caller established on it (see WithCaller), or the status error refusing
// the call (see StatusError). The one form serves a unary call and a stream
// alike, an authentication running once, ahead of the first message.
type Interceptor = grpcserver.Interceptor

// MethodStream is what the registration describes the rpc of a Stream
// action with in place of an HTTP method, Route answering it for such a
// call, and so the action word an authorization policy grants a stream by.
const MethodStream = grpcserver.MethodStream

// Method describes one rpc of a registered service the way the generated
// registration file declares it: its full name, whether its action declares
// Public(), and the HTTP method and route the same action is served at over
// HTTP, which Route answers and the interceptors of the modules judge a call
// by.
type Method = grpcserver.Method

// Register queues the service server serves to be registered on the
// listener through register, the RegisterXxxServiceServer function the
// protobuf plugin generated for it, the way the generated pb/pb.gen.go
// registers the service of every model declaring GRPC():
//
//	grpc.Register[NoteServiceServer](RegisterNoteServiceServer, NoteService{},
//		grpc.Method{Name: NoteService_CreateNote_FullMethodName, HTTPMethod: http.MethodPost, Route: "/api/notes"},
//		grpc.Method{Name: NoteService_GetNote_FullMethodName, HTTPMethod: http.MethodGet, Route: "/api/notes/:id"},
//	)
//
// S is the server interface the plugin generated, spelled out because the
// value registered is of the type serving it, which inference cannot tell
// apart from the interface. methods describe the service's rpcs. It runs
// at package initialization, before bootstrap starts the listeners, and
// panics once the listener runs.
func Register[S any](register func(grpc.ServiceRegistrar, S), server S, methods ...Method) {
	grpcserver.Register(func(r grpc.ServiceRegistrar) { register(r, server) }, methods...)
}
