package controller

import (
	"context"
	"fmt"
	"maps"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/grpcserver"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/status"
)

// This file holds the gRPC half of the actions, the counterpart of the HTTP
// handlers for the services the generated pb package registers. An HTTP
// handler binds the body and the route parameters of a request, runs the
// flow and writes its answer in the envelope; the call function of an rpc
// (see CreateCall and its kind, and ServiceCall) takes what the generated
// handler decoded from the request message — the route parameters, the
// query, the model or the payload — attaches the parameters to the call,
// runs the same flow on it and answers with the result, or with the status
// the failure maps to. The flow, the service resolution, the validation and
// the controller span are shared, which is what keeps an action behaving the
// same on both transports.

// Query is what the request message of an rpc carries for the query
// parameters of the same action over HTTP, as the List and Get requests the
// generator derives name them: the filters, orderings, pagination, cursor
// and expansion of a List, the expansion of a Get. It is rendered as the
// query string the HTTP listener would have received (see values) and read
// by the very same parsers, so both transports refuse the same requests
// with the same messages. The zero value of a field is the parameter not
// sent, proto3 having no presence for scalars: a Page of 0 is no _page, an
// empty SortBy no _sort_by.
//
// gRPC has no URL, so what the HTTP request carries in its path and query
// string the rpc's request message carries in its fields, the route
// parameters first, then these, a List request holding the fields of the
// controls the model reads and a custom one every field; the example on
// the public grpc.Query shows one query as an HTTP request, as the messages
// gg gen derives, in JSON and as a Query.
type Query struct {
	Filters     []Filter
	SortBy      []string
	Page        uint32
	Size        uint32
	CursorField string
	CursorValue string
	CursorNext  bool
	Expand      []string
	Depth       uint32
}

// Filter is one filter of a Query: Field names a column by its query name,
// Op is an operator of types.FilterOp and Values its value, several for in
// and notin, which take a list where the HTTP query takes a comma-separated
// one. With an operator it is the field[op]=value of the HTTP query,
// {Field: "age", Op: "gt", Values: []string{"20"}} being age[gt]=20; with
// none it is the bare key, field=value, the equality on the model's own
// field every model answers, while the operators need the model to declare
// model.Query, exactly as over HTTP.
type Filter struct {
	Field  string
	Op     string
	Values []string
}

// values renders q as the query string the HTTP listener parses into
// url.Values: a filter with an operator as field[op]=value, one without as
// field=value, the members of an in or notin joined by commas the way the
// HTTP parser splits them; the orderings joined by commas under _sort_by;
// page, size, the cursor and the expansion under their parameters when set.
// It refuses what the HTTP query could not carry: a filter given twice,
// since the HTTP listener refuses a repeated parameter; several values
// under an operator taking one; a member of an in holding a comma; and a
// filter without a value (see Filter.value).
//
// The query
//
//	Query{
//		Filters: []Filter{
//			{Field: "name", Values: []string{"alice"}},
//			{Field: "age", Op: "gt", Values: []string{"20"}},
//			{Field: "status", Op: "in", Values: []string{"active", "archived"}},
//		},
//		SortBy:      []string{"name", "created_at desc"},
//		Page:        2,
//		Size:        50,
//		CursorField: "id",
//		CursorValue: "r-1",
//		CursorNext:  true,
//		Expand:      []string{"children", "parent"},
//		Depth:       3,
//	}
//
// renders, written as a query string, as
//
//	name=alice&age[gt]=20&status[in]=active,archived&_sort_by=name,created_at desc&_page=2&_size=50&_cursor_field=id&_cursor_value=r-1&_cursor_next=true&_expand=children,parent&_depth=3
func (q Query) values() (url.Values, error) {
	values := make(url.Values)
	for _, f := range q.Filters {
		key := f.Field
		if f.Op != "" {
			key = f.Field + "[" + f.Op + "]"
		}
		if f.Field == "" {
			return nil, errors.Newf("filter %q: a field is required", key)
		}
		if strings.HasPrefix(f.Field, "_") {
			return nil, errors.Newf("filter %q: a field cannot start with an underscore, the framework's own parameters do", key)
		}
		if _, given := values[key]; given {
			return nil, errors.Newf("filter %q is given twice", key)
		}
		value, err := f.value(key)
		if err != nil {
			return nil, err
		}
		values[key] = []string{value}
	}
	for _, list := range []struct {
		name    string
		members []string
	}{{"sort_by", q.SortBy}, {"expand", q.Expand}} {
		for _, member := range list.members {
			if strings.Contains(member, ",") {
				return nil, errors.Newf("%s: a member cannot hold a comma, the members are joined by it", list.name)
			}
		}
	}
	if len(q.SortBy) > 0 {
		values.Set(consts.QUERY_SORT_BY, strings.Join(q.SortBy, ","))
	}
	if q.Page != 0 {
		values.Set(consts.QUERY_PAGE, strconv.FormatUint(uint64(q.Page), 10))
	}
	if q.Size != 0 {
		values.Set(consts.QUERY_SIZE, strconv.FormatUint(uint64(q.Size), 10))
	}
	if q.CursorField != "" {
		values.Set(consts.QUERY_CURSOR_FIELD, q.CursorField)
	}
	if q.CursorValue != "" {
		values.Set(consts.QUERY_CURSOR_VALUE, q.CursorValue)
	}
	if q.CursorNext {
		values.Set(consts.QUERY_CURSOR_NEXT, strconv.FormatBool(true))
	}
	if len(q.Expand) > 0 {
		values.Set(consts.QUERY_EXPAND, strings.Join(q.Expand, ","))
	}
	if q.Depth != 0 {
		values.Set(consts.QUERY_DEPTH, strconv.FormatUint(uint64(q.Depth), 10))
	}
	return values, nil
}

// value renders the values of f as the one value of key: the members of an
// in or notin joined by commas, so a member holding one is refused; the
// single value of any other operator, several being what only a repeated
// parameter would carry. A filter without a value, or with an empty one,
// filters by nothing and is refused: over HTTP an empty parameter means not
// filtering, but a filter the message spells out and leaves empty is a
// mistake to report.
//
// TODO: accept a member of an in or notin holding a comma. The parsers read
// the HTTP spelling of the list, members joined by commas, so the call has
// to spell it that way too; accepting any string takes a second input form
// of urlquery that is handed the members as a slice.
func (f Filter) value(key string) (string, error) {
	if len(f.Values) == 0 {
		return "", errors.Newf("filter %q has no value", key)
	}
	if slices.Contains(f.Values, "") {
		return "", errors.Newf("filter %q has an empty value", key)
	}
	if f.Op == string(types.FilterOpIn) || f.Op == string(types.FilterOpNotIn) {
		for _, v := range f.Values {
			if strings.Contains(v, ",") {
				return "", errors.Newf("filter %q: a value cannot hold a comma, the members are joined by it", key)
			}
		}
		return strings.Join(f.Values, ","), nil
	}
	if len(f.Values) > 1 {
		return "", errors.Newf("filter %q takes one value, %d given; in and notin take several", key, len(f.Values))
	}
	return f.Values[0], nil
}

// The messages a call refuses a request with, where the HTTP handler's
// wording speaks of a body or a route the call has none of.
const (
	// invalidMessageMsg answers a model or payload a validator other than
	// go-playground's refused, which names no field (see fieldViolations);
	// what failed stays in the log.
	invalidMessageMsg = "invalid request message"
	// missingIDMsg answers an item action whose message names no record.
	missingIDMsg = "id is required"
	// missingRecordMsg answers a Create, an Update or a Patch whose message
	// carries no record.
	missingRecordMsg = "record is required"
)

// call is one run of an action for an rpc: the call's context with the
// parameters attached and the controller span started, the span, the log of
// the action, and the service contexts built for the hooks and the service,
// checked when the call ends for a response a service tried to write.
type call struct {
	ctx   context.Context
	span  trace.Span
	log   types.Logger
	built []*types.ServiceContext
}

// beginCall attaches params and query to ctx as the parameters of the call
// (see grpcserver.WithParams) and starts the controller span on it, the way
// the HTTP handler starts one on the request, described by the method and
// route the call carries, the action's as the registration described it;
// the caller ends the span through end. A route parameter left empty, or
// carrying a slash, is reported once the call began, so the caller refuses
// it on the call, its span recording the refusal: over HTTP a parameter
// matches one segment of the path, so no request carries an empty one or
// one with a slash in it, while a message may carry either, and a service
// scoping its work by the parameter would then scope it by nothing, or by a
// path of its own. The parameter carrying the id of an item action (see
// action.itemParam) is left to the caller when empty: the generated handler
// fills it from the id field of the message, so an empty one is an empty id,
// which the caller refuses by that field's name (see missingID).
func (a *action[M, REQ, RSP]) beginCall(ctx context.Context, params map[string]string, query url.Values) (*call, error) {
	ctx = grpcserver.WithParams(ctx, params, query)
	reqMeta := requestctx.FromContext(ctx)
	spanCtx, span := a.startSpan(ctx, reqMeta.Method(), reqMeta.Route())
	c := &call{ctx: spanCtx, span: span, log: logger.Controller.WithContext(ctx, a.phase)}
	for _, name := range slices.Sorted(maps.Keys(params)) {
		if params[name] == "" && name != a.itemParam {
			return c, errors.Newf("route parameter %q is required", name)
		}
		if strings.Contains(params[name], "/") {
			return c, errors.Newf("route parameter %q must not contain \"/\"", name)
		}
	}
	return c, nil
}

// beginQueryCall is beginCall for an action reading a query, a List or a
// Get: the query is rendered as the HTTP query string (see Query.values)
// and attached with the parameters. A query the HTTP listener could not
// carry is reported once the call began, like an empty parameter, so the
// caller refuses it on the call.
func (a *action[M, REQ, RSP]) beginQueryCall(ctx context.Context, params map[string]string, query Query) (*call, error) {
	values, queryErr := query.values()
	c, err := a.beginCall(ctx, params, values)
	if err != nil {
		return c, err
	}
	return c, queryErr
}

// serviceContext is the serviceContextFunc of the call: its service
// contexts carry no HTTP request, and are kept for finish to check what
// the hooks' own checks did not, a service method's calls.
func (c *call) serviceContext(ctx context.Context, phase consts.Phase) *types.ServiceContext {
	sc := types.NewServiceContext(nil, ctx, phase)
	c.built = append(c.built, sc)
	return sc
}

// end ends the controller span.
func (c *call) end() { c.span.End() }

// refuse answers a request the action cannot run — a query the listener
// could not carry, a model or payload failing validation, a message naming
// no record — with the status answer maps to (see grpcserver.StatusError),
// err logged and recorded on the span the way the HTTP handler treats a
// bind failure.
func (c *call) refuse(answer, err error) error {
	c.log.Errorz("request message rejected", zap.Error(err))
	gstotel.RecordError(c.span, err)
	return grpcserver.StatusError(answer)
}

// invalid refuses the request for err with the message err carries, the
// way the HTTP handler answers a bind failure (see invalidArgument).
func (c *call) invalid(err error) error {
	return c.refuse(invalidArgument(err), err)
}

// invalidMessage refuses a model or payload the validator refused (err),
// naming the fields refused the way the HTTP handler does (see
// clientSafeBindError), and with invalidMessageMsg when the validator
// names none.
func (c *call) invalidMessage(err error) error {
	if violations := fieldViolations(err, ""); len(violations) > 0 {
		return c.refuse(types.NewInvalidFields(violations, err), err)
	}
	return c.refuse(badRequest(invalidMessageMsg), err)
}

// invalidItemMessage is invalidMessage for the item at index i of a batch
// validated on its own, the fields it names carrying the item in front (see
// clientSafeItemBindError).
func (c *call) invalidItemMessage(i int, err error) error {
	if violations := fieldViolations(err, "items["+strconv.Itoa(i)+"]."); len(violations) > 0 {
		return c.refuse(types.NewInvalidFields(violations, err), err)
	}
	return c.invalidMessage(err)
}

// missingID refuses a call of an item action whose message names no record,
// the way the HTTP handler refuses a request whose route parameter is
// absent.
func (c *call) missingID() error {
	return c.refuse(badRequest(missingIDMsg), errors.New(missingIDMsg))
}

// missingRecord refuses a call whose message carries no record, the way the
// HTTP handler refuses a request without a body: an absent record would
// create a zero one or replace the stored one by it.
func (c *call) missingRecord() error {
	return c.refuse(badRequest(missingRecordMsg), errors.New(missingRecordMsg))
}

// fail answers a flow's failure, which the flow logged and recorded already,
// with the status it maps to (see grpcserver.StatusError), or with the
// status of the call's context once the call ended (see ended).
func (c *call) fail(err error) error {
	if ended := c.ended(); ended != nil {
		return ended
	}
	return grpcserver.StatusError(err)
}

// failService answers a delegated service's error the way the HTTP handler
// does: logged and recorded on the span, then the status it maps to (see
// grpcserver.StatusError). A call that ended is answered with the status of
// its context instead, the error unlogged (see ended).
func (c *call) failService(err error) error {
	if ended := c.ended(); ended != nil {
		return ended
	}
	c.log.Errorz("service operation failed", zap.Error(err))
	gstotel.RecordError(c.span, err)
	return grpcserver.StatusError(err)
}

// ended returns the status of the call's context once the call ended, nil
// while it goes on: Canceled once the client canceled the call or went away,
// DeadlineExceeded once the deadline it set passed. A call ending this way is
// the client's doing, or the listener's stop, which answers a stream it ended
// Unavailable in this status's place (see grpcserver); it is not a failure of
// the flow or the service, whatever they returned then — a database access
// cut short, a Send or Recv of a stream the client stopped — so the failure
// is answered as the context's status, which the access log records, and not
// logged as a failure of the server's.
func (c *call) ended() error {
	if err := c.ctx.Err(); err != nil {
		return status.FromContextError(err).Err()
	}
	return nil
}

// httpOnlyMethodCalled returns the error of a service that called a method
// of sc only an HTTP request can serve — wrote a body, a stream or a cookie
// it believes answered, read a cookie, a form value or a file that was never
// there — while sc carried no HTTP request (see types.HTTPOnlyMethodCalled),
// naming the methods, and nil when it called none. A call answers Internal
// for it, right after the hook that called it (see traceServiceHook) or
// once its service method returned (see finish); gg check reports the call
// at generation time, this is the transport's own refusal. A context of an
// HTTP request carries the request, so its hooks never trip it.
func httpOnlyMethodCalled(sc *types.ServiceContext) error {
	if !types.HTTPOnlyMethodCalled(sc) {
		return nil
	}
	return errors.Newf("the service called a method of its context only an HTTP request can serve: an action served over gRPC must not call %s", strings.Join(types.HTTPOnlyMethods, ", "))
}

// finish ends a call whose flow or service returned, checking the service
// contexts it built the way traceServiceHook checks each hook's (see
// httpOnlyMethodCalled): what a service method called on its context is
// checked here, once the method returned. nil otherwise.
func (c *call) finish() error {
	for _, sc := range c.built {
		err := httpOnlyMethodCalled(sc)
		if err == nil {
			continue
		}
		c.log.Errorz("service operation failed", zap.Error(err))
		gstotel.RecordError(c.span, err)
		return grpcserver.StatusError(err)
	}
	return nil
}

// answer finishes c (see finish) and returns result, or the status finish
// answered instead.
func answer[T any](c *call, result T) (T, error) {
	if err := c.finish(); err != nil {
		var zero T
		return zero, err
	}
	return result, nil
}

// ServiceCall returns the call of the phase service's method for the action
// of phase on route, for the generated handler of the rpc of an action
// declaring a Payload or Result of its own: the counterpart of the
// handler serviceHandler returns when M, REQ and RSP differ. Given the route
// parameters, the query of a List or Get, and the payload the request
// message decoded into, the call validates the payload the way the handler
// validates a bound body — not for a List or Get, whose HTTP request binds
// no body — runs the method in its service span on a service context
// answering the parameters, the query and the caller, and answers with its
// result, or with the status its error maps to (see grpcserver.StatusError).
// phase must be one gRPC serves: the HTTP-only actions have no rpc, so their
// phase panics here, as the route registers.
func ServiceCall[M types.Model, REQ types.Request, RSP types.Response](phase consts.Phase, route string) func(ctx context.Context, params map[string]string, query Query, req REQ) (RSP, error) {
	invoke := serviceMethod[M, REQ, RSP](phase)
	a := newAction[M, REQ, RSP](route, phase)
	binds := phase != consts.List && phase != consts.Get
	return func(ctx context.Context, params map[string]string, query Query, req REQ) (RSP, error) {
		var zero RSP
		c, err := a.beginQueryCall(ctx, params, query)
		defer c.end()
		if err != nil {
			return zero, c.invalid(err)
		}
		a.normalizeRequest(&req)
		if binds {
			if err = validateRequest(req); err != nil {
				return zero, c.invalidMessage(err)
			}
		}
		rsp, err := a.traceServiceOperation(c.ctx, phase, func(spanCtx context.Context) (RSP, error) {
			return invoke(a.service(), c.serviceContext(spanCtx, phase), req)
		})
		if err != nil {
			return zero, c.failService(err)
		}
		return answer(c, rsp)
	}
}

// PatchItem readies the record of the item at index i of a batch patch, what
// the generated handler of a PatchMany rpc reads each item it carries
// through, the item being the request of a single Patch: the item names its
// record by id, written onto the record it carries in place of whatever id
// the record names, the way a patch call patches the record its message
// names whatever id the record carries (see PatchCall); and the route
// parameters it carries, keyed as the request's params are, may be left
// empty or repeat the request's. An item naming no id, or naming a
// parameter otherwise than the request does, is refused with
// InvalidArgument, the way a batch request whose sub-request names another
// parent must fail (AIP-234), the way a call's refusal does (see
// grpcserver.StatusError); an item carrying no record is answered as it is,
// for the call to refuse (see PatchManyCall). The public grpc.PatchItem
// forwards to it.
func PatchItem[M types.Model](i int, params, itemParams map[string]string, id string, m M) (M, error) {
	invalid := func(format string, args ...any) error {
		return grpcserver.StatusError(badRequest(fmt.Sprintf(format, args...)))
	}
	if name, value, differs := disagreeingParam(params, itemParams); differs {
		return m, invalid("item %d names the %s parameter %q, the request names %q", i, name, value, params[name])
	}
	if id == "" {
		return m, invalid("item %d names no id", i)
	}
	if reflect.ValueOf(m).IsNil() {
		return m, nil
	}
	// SetID keeps an id already set, so the one the record carries is
	// cleared first.
	m.ClearID()
	m.SetID(id)
	return m, nil
}

// disagreeingParam returns the first route parameter, by name, that given
// names otherwise than params, the parameters of the request, with the
// value given names it: a parameter given leaves empty or names as params
// does agrees. A message or item of a request carrying the parameters of
// its own may repeat the request's, never contradict them.
func disagreeingParam(params, given map[string]string) (name, value string, differs bool) {
	for _, name := range slices.Sorted(maps.Keys(given)) {
		if value := given[name]; value != "" && value != params[name] {
			return name, value, true
		}
	}
	return "", "", false
}
