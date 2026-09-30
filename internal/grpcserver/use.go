package grpcserver

import (
	"context"
	"net/url"
	"slices"

	middleware "github.com/grpc-ecosystem/go-grpc-middleware/v2"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/selector"
	"github.com/hydroan/gst/internal/requestctx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

// Method describes one rpc of a registered service the way the generated
// registration file declares it: its full name, whether its action declares
// Public(), and the HTTP method and route the same action is served at over
// HTTP, which the interceptors of the modules judge a call by the way their
// middleware judges a request.
type Method struct {
	// Name is the full method name, "/app.RecordService/ListRecord".
	Name string
	// HTTPMethod and Route are the HTTP method and the route pattern of the
	// same action, "GET" and "/api/records/:id"; for the rpc of a Stream
	// action, served over gRPC alone, HTTPMethod is MethodStream and Route
	// the path the action is declared at, "/api/feeds/watch", which nothing
	// serves over HTTP.
	HTTPMethod string
	Route      string
	// Public marks the action as one declaring Public(): the interceptors
	// UseAuth queued leave the method alone.
	Public bool
}

// MethodStream is what the registration describes the rpc of a Stream
// action with in place of an HTTP method, and so the action word an
// authorization policy grants a stream by: a stream has no HTTP method, and
// a policy written for GET or POST must not let one through. The public
// grpc.MethodStream forwards to it.
const MethodStream = "STREAM"

// Interceptor is an interceptor a project or a copied module mounts through
// Use or UseAuth: it reads what the call carries, its metadata, its caller,
// the action it maps to (see Route), and returns the context the call goes
// on with, the caller established on it (see WithCaller), or the status
// error refusing the call. The one form serves a unary call and a stream
// alike, which is why an interceptor here transforms the context rather
// than wrapping the handler the way a grpc.UnaryServerInterceptor does: an
// authentication runs once, ahead of the first message either way. The
// public grpc.Interceptor forwards to it.
type Interceptor func(ctx context.Context) (context.Context, error)

var (
	// commonInterceptors and authInterceptors are the interceptors Use and
	// UseAuth queued, in order: the project's, run inside the framework's own
	// chain on every method and on the non-public methods respectively.
	commonInterceptors []Interceptor
	authInterceptors   []Interceptor
	// methods are the rpcs Register described, keyed by their full name.
	methods map[string]Method
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

// unguardedMethods lists the registered methods not declared public, in
// order of their names: the ones the auth interceptors guard, or would.
func unguardedMethods() []string {
	names := make([]string, 0, len(methods))
	for name, m := range methods {
		if !m.Public {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// ownServices are the services the server registers for itself (see Run),
// the health service and the reflection service in its two versions, which
// the project's interceptors leave alone, the common and the auth ones
// alike: they are the framework's, not the project's actions, and the
// callers of either present no credentials — a Kubernetes gRPC probe or a
// balancer checking the health service, grpcurl listing the services
// through reflection — the way the HTTP listener's probes run outside the
// middleware a project registers and take no authentication. Reflection
// exposes the schema alone, what the committed .proto files carry; [grpc]
// reflection turns it off where that is too much.
var ownServices = map[string]bool{
	grpc_health_v1.Health_ServiceDesc.ServiceName:                    true,
	grpc_reflection_v1.ServerReflection_ServiceDesc.ServiceName:      true,
	grpc_reflection_v1alpha.ServerReflection_ServiceDesc.ServiceName: true,
}

// requiresAuth reports whether the call of fullMethod, an rpc of service,
// is one the auth interceptors run on, which is what the RequiresAuth of
// its request metadata says (see enterCall): a call of a method not
// declared public, the server's own services aside.
func requiresAuth(service, fullMethod string) bool {
	return !ownServices[service] && !methods[fullMethod].Public
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
// returns, or the call is refused with its error.
func unaryOf(ic Interceptor) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx, err := ic(ctx)
		if err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// streamOf runs ic on a stream, once, ahead of the first message: the
// handler gets the stream on the context ic returns, or the stream is
// refused with its error.
func streamOf(ic Interceptor) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx, err := ic(ss.Context())
		if err != nil {
			return err
		}
		return handler(srv, withStreamContext(ctx, ss))
	}
}

// withStreamContext returns ss carrying ctx as its context, what the
// handler and the calls downstream read the call's metadata and caller
// from.
func withStreamContext(ctx context.Context, ss grpc.ServerStream) grpc.ServerStream {
	wrapped := middleware.WrapServerStream(ss)
	wrapped.WrappedContext = ctx
	return wrapped
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
// attached and the method as registered, and what comes after it adds: the
// caller an interceptor establishes, the parameters the handler attaches.
// It is the one place the access-log entry written when the call ends reads
// the caller from, since a context cannot carry a value back up the chain,
// and what every rebuild of the request metadata starts from, so that what
// one addition attached survives the next.
type callRecord struct {
	fields requestctx.Fields
	method Method
	caller Caller
}

// requestFields returns the fields of the call's request metadata: what
// requestScope recorded and WithParams added, and the caller as established
// so far.
func (c *callRecord) requestFields() requestctx.Fields {
	fields := c.fields
	fields.Username = c.caller.Username
	fields.UserID = c.caller.UserID
	fields.SessionID = c.caller.SessionID
	fields.TenantID = c.caller.TenantID
	return fields
}

// Route returns the HTTP method and route the registration described the
// call's rpc with, "GET" and "/api/records" for a call of ListRecord: what
// an interceptor judging the call by the action, the way the module
// middleware judges a request, asks for. Both are empty outside a call and
// for a method registered without them, the health and reflection services'
// among them. The public grpc.Route forwards to it.
func Route(ctx context.Context) (httpMethod, route string) {
	if c, ok := ctx.Value(callRecordKey{}).(*callRecord); ok {
		return c.method.HTTPMethod, c.method.Route
	}
	return "", ""
}

// CallerOf returns the caller WithCaller established for the call, the zero
// Caller before one is established and outside a call. The public
// grpc.CallerOf forwards to it.
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
// the public grpc.WithCaller forwards to it.
func WithCaller(ctx context.Context, caller Caller) context.Context {
	c, ok := ctx.Value(callRecordKey{}).(*callRecord)
	if !ok {
		c = &callRecord{}
	}
	c.caller = caller
	return requestctx.WithMetadata(ctx, requestctx.New(c.requestFields()))
}

// WithParams returns ctx with params and query as the parameters of the
// call: the request metadata on the returned context answers Param and Query
// with them, so the ServiceContext built on it and the flows read the route
// parameters and the query the way they read a request's, beside what the
// call already carries, the method as route and the caller as established.
// The call functions of the controller attach what the request message
// carries for them, its leading fields standing for the route's parameters
// and the query fields of a List or Get, before running the action.
func WithParams(ctx context.Context, params map[string]string, query url.Values) context.Context {
	c, ok := ctx.Value(callRecordKey{}).(*callRecord)
	if !ok {
		c = &callRecord{}
	}
	c.fields.Params = params
	c.fields.Query = query
	return requestctx.WithMetadata(ctx, requestctx.New(c.requestFields()))
}
