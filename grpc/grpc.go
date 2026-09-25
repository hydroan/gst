// Package grpc is the gRPC listener's API for a project's own code: what an
// interceptor of the project or of a copied module reads and establishes
// about a call — its caller, the HTTP action it maps to, the status a
// failure answers with. The listener itself, its chain of interceptors and
// the registration of the generated services are the framework's own; the
// interceptors a project mounts are registered through package interceptor,
// the counterpart of package middleware.
package grpc

import (
	"context"
	"strings"

	"github.com/hydroan/gst/internal/grpcserver"
	"google.golang.org/grpc/metadata"
)

// The metadata a call's credential arrives in, in the lowercase gRPC
// metadata keeps its keys in: authorizationKey carries it as
// "<bearerScheme> <credential>", the Authorization header of the HTTP
// listener.
const (
	authorizationKey = "authorization"
	bearerScheme     = "Bearer"
)

// Bearer returns the credential the call's authorization metadata carries
// as "Bearer <credential>" — a token for JwtAuth, a session id for the IAM
// session interceptor — and whether it carries one: the check
// jwt.ParseTokenFromHeader makes of the Authorization header. Any other
// scheme, or no authorization metadata at all, is no credential.
func Bearer(ctx context.Context) (credential string, ok bool) {
	md, _ := metadata.FromIncomingContext(ctx)
	values := md.Get(authorizationKey)
	if len(values) == 0 {
		return "", false
	}
	scheme, credential, found := strings.Cut(values[0], " ")
	if !found || scheme != bearerScheme {
		return "", false
	}
	return credential, true
}

// Caller is who a call is made by, as an authentication interceptor
// established it: the fields the HTTP authentication middleware sets on the
// gin context.
type Caller = grpcserver.Caller

// WithCaller returns ctx with caller established as who is calling: the
// request metadata the service context and the flows read names the
// caller, and so does the call's access-log entry. An authentication
// interceptor calls it once it has verified who is calling, and hands the
// returned context on to the handler; a gRPC context cannot be written the
// way a gin context can, which is why the caller travels this way.
func WithCaller(ctx context.Context, caller Caller) context.Context {
	return grpcserver.WithCaller(ctx, caller)
}

// CallerOf returns the caller WithCaller established for the call, the zero
// Caller before one is established.
func CallerOf(ctx context.Context) Caller {
	return grpcserver.CallerOf(ctx)
}

// Route returns the HTTP method and route the call's action is served at
// over HTTP, "GET" and "/api/records" for a call of ListRecord, as the
// generated registration described the rpc: what an interceptor judging the
// call by the action, the way its middleware counterpart judges a request,
// asks for. Both are empty outside a call and for the framework's own
// services, health and reflection.
func Route(ctx context.Context) (httpMethod, route string) {
	return grpcserver.Route(ctx)
}

// StatusError returns the status error a call answers err with: a service
// error (see service.NewError) answers with its own status and message,
// mapped to the gRPC code the way the HTTP listener maps it to a response;
// any other error answers Internal with a fixed message, its text kept out
// of the answer, so log it before mapping it. nil stays nil.
func StatusError(err error) error {
	return grpcserver.StatusError(err)
}
