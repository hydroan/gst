package grpc

import (
	"context"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/controller"
	"github.com/hydroan/gst/internal/types"
)

// This file holds the call functions the generated handlers of a model's
// rpcs run the actions through: one per standard action, CreateCall and its
// kind, and ServiceCall for an action with a payload or result of its own.
// Each returns the call of M's action on route, the raw route the generated
// router registers the action under, built once at package initialization
// and shared by every call. Given the route parameters the request message
// carries, keyed as the route names them, and what else it decoded — the
// id, the model, the items, the update mask, the query — a call validates
// the input the way the HTTP handler validates a bound body, runs the very
// flow of the HTTP action, hooks, database access and operation log alike,
// and answers with the result, or with the status the failure maps to, the
// way StatusError maps a service error.

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
// name the fields to apply as the message names them and must name at least
// one, it answers with the record patched.
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
