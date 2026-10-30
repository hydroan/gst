package grpc

import (
	"context"
	"strings"

	"github.com/hydroan/gst/internal/grpcserver"
	"google.golang.org/grpc/metadata"
)

// This file holds what a call's context answers and what is established on it:
// the credential the call presents, the caller WithCaller names, and the HTTP
// action the call maps to.

// The metadata a call's credential arrives in, in the lowercase gRPC
// metadata keeps its keys in: keyAuthorization carries it as
// "<bearerScheme> <credential>", the Authorization header of the HTTP
// listener.
const (
	keyAuthorization = "authorization"
	bearerScheme     = "Bearer"
)

// Bearer returns the credential the call's authorization metadata carries
// as "Bearer <credential>", a token or a session id, and whether it carries
// one: the check jwt.ParseTokenFromHeader makes of the Authorization
// header. Any other scheme, or no authorization metadata at all, is no
// credential.
func Bearer(ctx context.Context) (credential string, ok bool) {
	md, _ := metadata.FromIncomingContext(ctx)
	values := md.Get(keyAuthorization)
	if len(values) == 0 {
		return "", false
	}
	scheme, credential, found := strings.Cut(values[0], " ")
	if !found || scheme != bearerScheme {
		return "", false
	}
	return credential, true
}

// Caller is who a call is made by, as WithCaller established it: the user by
// name and id, the session and the tenant.
type Caller = grpcserver.Caller

// WithCaller returns ctx with caller established as who is calling: the
// request metadata the service context and the flows read names the
// caller, and so does the call's access-log entry. Hand the returned context on
// to the handler once who is calling has been verified; a gRPC context cannot
// be written the way a gin context can, which is why the caller travels this
// way.
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
// generated registration described the rpc (see Method). Both are empty outside
// a call and for the framework's own services, health and reflection.
func Route(ctx context.Context) (httpMethod, route string) {
	return grpcserver.Route(ctx)
}
