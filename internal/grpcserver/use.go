package grpcserver

import (
	"context"
	"slices"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/selector"
	"github.com/hydroan/gst/internal/requestctx"
	"google.golang.org/grpc"
)

// Method describes one rpc of a registered service the way the generated
// registration file declares it: its full name, whether its action declares
// Public(), and the HTTP method and route the same action is served at over
// HTTP, which the interceptors of the modules judge a call by the way their
// middleware judges a request.
type Method struct {
	// Name is the full method name, "/app.RecordService/ListRecord".
	Name string
	// Public marks the action as one declaring Public(): the interceptors
	// UseAuth queued leave the method alone.
	Public bool
	// HTTPMethod and Route are the HTTP method and the route pattern of the
	// same action, "GET" and "/api/records/:id".
	HTTPMethod string
	Route      string
}

var (
	// commonInterceptors and authInterceptors are the interceptors Use and
	// UseAuth queued, in order: the project's, run inside the framework's own
	// chain on every method and on the non-public methods respectively.
	commonInterceptors []grpc.UnaryServerInterceptor
	authInterceptors   []grpc.UnaryServerInterceptor
	// methods are the rpcs Register described, keyed by their full name.
	methods map[string]Method
)

// Use queues interceptors to run on every call, after the framework's own
// chain and before the ones UseAuth queued, in the order given, the way the
// HTTP listener runs the middleware Register adds on every route. The public
// interceptor.Register forwards to it. Like Register it runs at package
// initialization; queuing once the server runs would intercept nothing, so
// it panics.
func Use(interceptors ...grpc.UnaryServerInterceptor) {
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
func UseAuth(interceptors ...grpc.UnaryServerInterceptor) {
	mu.Lock()
	defer mu.Unlock()
	if started.Load() {
		panic("grpcserver: UseAuth after the server started; register interceptors at package initialization")
	}
	authInterceptors = append(authInterceptors, interceptors...)
}

// projectInterceptors returns the interceptors Use and UseAuth queued, in
// the order they run: the common ones, then the auth ones, each behind a
// selector that leaves the public methods alone.
func projectInterceptors() []grpc.UnaryServerInterceptor {
	chain := slices.Clone(commonInterceptors)
	guarded := selector.MatchFunc(func(_ context.Context, meta interceptors.CallMeta) bool {
		return !methods[meta.FullMethod()].Public
	})
	for _, auth := range authInterceptors {
		chain = append(chain, selector.UnaryServerInterceptor(auth, guarded))
	}
	return chain
}

// Caller is who a call is made by, as an authentication interceptor
// established it: the fields the HTTP listener's authentication middleware
// sets on the gin context.
type Caller struct {
	Username  string
	UserID    string
	SessionID string
	TenantID  string
}

// callRecordKey keys the call record requestScope stores on the context.
type callRecordKey struct{}

// callRecord is what requestScope knows of a call, the request metadata it
// attached and the method as registered, and what the interceptors after it
// add, the caller: the one place the access-log entry written when the call
// ends reads the caller from, since a context cannot carry a value back up
// the chain.
type callRecord struct {
	fields requestctx.Fields
	method Method
	caller Caller
}

// Route returns the HTTP method and route the registration described the
// call's rpc with, "GET" and "/api/records" for a call of ListRecord: what
// an interceptor judging the call by the action, the way the module
// middleware judges a request, asks for. Both are empty outside a call and
// for a method registered without them, the health and reflection services'
// among them. The public interceptor.Route forwards to it.
func Route(ctx context.Context) (httpMethod, route string) {
	if c, ok := ctx.Value(callRecordKey{}).(*callRecord); ok {
		return c.method.HTTPMethod, c.method.Route
	}
	return "", ""
}

// CallerOf returns the caller WithCaller established for the call, the zero
// Caller before one is established and outside a call. The public
// interceptor.CallerOf forwards to it.
func CallerOf(ctx context.Context) Caller {
	if c, ok := ctx.Value(callRecordKey{}).(*callRecord); ok {
		return c.caller
	}
	return Caller{}
}

// WithCaller returns ctx with caller established as who is calling: the
// request metadata on the returned context names the caller, so the
// ServiceContext built on it and the flows do, and the access-log entry of
// the call names the caller as well. An authentication interceptor calls it
// once it has verified who is calling, and hands the returned context on;
// the public interceptor.WithCaller forwards to it.
func WithCaller(ctx context.Context, caller Caller) context.Context {
	var fields requestctx.Fields
	if c, ok := ctx.Value(callRecordKey{}).(*callRecord); ok {
		c.caller = caller
		fields = c.fields
	}
	fields.Username = caller.Username
	fields.UserID = caller.UserID
	fields.SessionID = caller.SessionID
	fields.TenantID = caller.TenantID
	return requestctx.WithMetadata(ctx, requestctx.New(fields))
}
