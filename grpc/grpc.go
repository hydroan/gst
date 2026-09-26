// Package grpc is the gRPC listener's API for a project's own code: what an
// interceptor of the project or of a copied module reads and establishes
// about a call — its caller, the HTTP action it maps to, the status a
// failure answers with — and what the generated pb package registers and
// runs the services of the models declaring GRPC() with: Register, and the
// call functions of the actions, CreateCall and its kind for the standard
// actions and ServiceCall for the actions with a payload or result of their
// own. The listener itself and its chain of interceptors are the
// framework's own; the interceptors a project mounts are registered through
// package interceptor, the counterpart of package middleware.
package grpc

import (
	"context"
	"strings"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/controller"
	"github.com/hydroan/gst/internal/grpcserver"
	"github.com/hydroan/gst/internal/types"
	"google.golang.org/grpc"
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

// Query is what the request message of a List or Get rpc carries for the
// query parameters of the same action over HTTP, as the generated messages
// name them: the filters, orderings, pagination, cursor and expansion of a
// List, the expansion of a Get. The call functions read it exactly as the
// HTTP listener reads the query string, so both transports refuse the same
// requests; the zero value of a field is the parameter not sent.
type Query = controller.Query

// Filter is one filter of a Query, the field[op]=value of the HTTP query;
// without an operator it is the bare key, field=value.
type Filter = controller.Filter

// The call functions of the standard actions, CreateCall and its kind, one
// per action a model's service serves: each returns the call of M's action
// on route, the raw route the generated router registers the action under,
// for the generated handler of the rpc. Given the route parameters the
// request message carries, keyed as the route names them, and what else it
// decoded — the id, the model, the items, the update mask, the query — a
// call validates the input the way the HTTP handler validates a bound body,
// runs the very flow of the HTTP action, hooks, database access and
// operation log alike, and answers with the result, or with the status the
// failure maps to, the way StatusError maps a service error. The functions
// are built at package initialization, one per rpc, and shared by every
// call.

// CreateCall returns the create call of M on route: given the route
// parameters and the model to create, it answers with the model created.
func CreateCall[M types.Model](route string) func(ctx context.Context, params map[string]string, m M) (M, error) {
	return controller.CreateCall[M](route)
}

// GetCall returns the get call of M on route: given the route parameters,
// the id and the expansion query, it answers with the model found.
func GetCall[M types.Model](route string) func(ctx context.Context, params map[string]string, id string, query Query) (M, error) {
	return controller.GetCall[M](route)
}

// ListCall returns the list call of M on route: given the route parameters
// and the query, it answers with the items of the page and the total, 0
// under cursor pagination.
func ListCall[M types.Model](route string) func(ctx context.Context, params map[string]string, query Query) ([]M, int, error) {
	return controller.ListCall[M](route)
}

// UpdateCall returns the update call of M on route: given the route
// parameters, the id and the replacement, it answers with the replacement as
// stored.
func UpdateCall[M types.Model](route string) func(ctx context.Context, params map[string]string, id string, m M) (M, error) {
	return controller.UpdateCall[M](route)
}

// PatchCall returns the patch call of M on route: given the route
// parameters, the id, the values and the paths of the update mask, which
// name the fields to apply as the message names them, it answers with the
// record patched.
func PatchCall[M types.Model](route string) func(ctx context.Context, params map[string]string, id string, m M, paths []string) (M, error) {
	return controller.PatchCall[M](route)
}

// DeleteCall returns the delete call of M on route: given the route
// parameters and the id, it answers nothing.
func DeleteCall[M types.Model](route string) func(ctx context.Context, params map[string]string, id string) error {
	return controller.DeleteCall[M](route)
}

// CreateManyCall returns the batch create call of M on route: given the
// route parameters and the items, it answers with the items created.
func CreateManyCall[M types.Model](route string) func(ctx context.Context, params map[string]string, items []M) ([]M, error) {
	return controller.CreateManyCall[M](route)
}

// UpdateManyCall returns the batch update call of M on route: given the
// route parameters and the items, it answers with the items as stored.
func UpdateManyCall[M types.Model](route string) func(ctx context.Context, params map[string]string, items []M) ([]M, error) {
	return controller.UpdateManyCall[M](route)
}

// PatchManyCall returns the batch patch call of M on route: given the route
// parameters, the items and the paths of each item's update mask, in order,
// it answers with the records patched.
func PatchManyCall[M types.Model](route string) func(ctx context.Context, params map[string]string, items []M, paths [][]string) ([]M, error) {
	return controller.PatchManyCall[M](route)
}

// DeleteManyCall returns the batch delete call of M on route: given the
// route parameters and the ids, it answers nothing.
func DeleteManyCall[M types.Model](route string) func(ctx context.Context, params map[string]string, ids []string) error {
	return controller.DeleteManyCall[M](route)
}

// ServiceCall returns the call of the phase service's method for the action
// of phase on route, for the generated handler of the rpc of an action
// declaring a Payload or Result of its own: given the route parameters, the
// query of a List or Get and the payload the request message decoded into,
// it validates the payload the way the HTTP handler validates a bound body,
// runs the method on a service context answering the parameters, the query
// and the caller, and answers with its result, or with the status its error
// maps to. phase is one of the actions gRPC serves; the phase of an HTTP-only
// action panics, as the route registers.
func ServiceCall[M types.Model, REQ types.Request, RSP types.Response](phase consts.Phase, route string) func(ctx context.Context, params map[string]string, query Query, req REQ) (RSP, error) {
	return controller.ServiceCall[M, REQ, RSP](phase, route)
}
