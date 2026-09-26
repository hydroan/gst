package controller

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/grpcserver"
	"github.com/hydroan/gst/internal/requestctx"
	. "github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// This file holds the gRPC half of the handlers, the counterpart of the
// factories for the services the generated pb package registers. An HTTP
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
// and notin. With an operator it is the field[op]=value of the HTTP query;
// with none it is the bare key, field=value, the equality on the model's own
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
// under an operator taking one; a member of an in holding a comma.
func (q Query) values() (url.Values, error) {
	values := make(url.Values)
	for _, f := range q.Filters {
		key := f.Field
		if f.Op != "" {
			key = f.Field + "[" + f.Op + "]"
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
// parameter would carry. No value renders as the empty value, which the
// parsers read as not filtering, the way they read an empty parameter.
func (f Filter) value(key string) (string, error) {
	if f.Op == string(types.FilterOpIn) || f.Op == string(types.FilterOpNotIn) {
		for _, v := range f.Values {
			if strings.Contains(v, ",") {
				return "", errors.Newf("filter %q: a value cannot hold a comma, the members are joined by it", key)
			}
		}
		return strings.Join(f.Values, ","), nil
	}
	switch len(f.Values) {
	case 0:
		return "", nil
	case 1:
		return f.Values[0], nil
	default:
		return "", errors.Newf("filter %q takes one value, %d given; in and notin take several", key, len(f.Values))
	}
}

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
// the HTTP handler starts one on the request, described by what the call
// carries for the method and path, POST and the full method; the caller
// ends the span through end.
func (meta *factoryMeta[M, REQ, RSP]) beginCall(ctx context.Context, params map[string]string, query url.Values) *call {
	ctx = grpcserver.WithParams(ctx, params, query)
	reqMeta := requestctx.FromContext(ctx)
	spanCtx, span := meta.startSpan(ctx, reqMeta.Method(), reqMeta.Route())
	return &call{ctx: spanCtx, span: span, log: logger.Controller.WithContext(ctx, meta.phase)}
}

// beginQueryCall is beginCall for an action reading a query, a List or a
// Get: the query is rendered as the HTTP query string (see Query.values)
// and attached with the parameters. A query the HTTP listener could not
// carry is reported once the call began, so the caller refuses it on the
// call, its span recording the refusal.
func (meta *factoryMeta[M, REQ, RSP]) beginQueryCall(ctx context.Context, params map[string]string, query Query) (*call, error) {
	values, err := query.values()
	return meta.beginCall(ctx, params, values), err
}

// serviceContext is the serviceContextFunc of the call: its service
// contexts carry no HTTP request, and are kept for finish to check.
func (c *call) serviceContext(ctx context.Context, phase consts.Phase) *types.ServiceContext {
	sc := types.NewServiceContext(nil, ctx, phase)
	c.built = append(c.built, sc)
	return sc
}

// end ends the controller span.
func (c *call) end() { c.span.End() }

// refuse answers a request the action cannot run — a query the listener
// could not carry, a model or payload failing validation, a message naming
// no record — with the status coder maps to, logged and recorded on the
// span the way the HTTP handler treats a bind failure.
func (c *call) refuse(coder types.Coder, err error) error {
	c.log.Errorz("request message rejected", zap.Error(err))
	gstotel.RecordError(c.span, err)
	return grpcserver.StatusOfCoder(coder)
}

// invalid refuses the request for err with the message err carries, the
// way the HTTP handler answers CodeInvalidParam.WithErr.
func (c *call) invalid(err error) error {
	return c.refuse(CodeInvalidParam.WithErr(err), err)
}

// missingID refuses a call of an item action whose message names no record,
// the way the HTTP handler refuses a request whose route parameter is
// absent.
func (c *call) missingID() error {
	return c.refuse(CodeInvalidParam.WithMsg(missingRouteParamMsg), errors.New(missingRouteParamMsg))
}

// fail answers a flow's failure, which the flow logged and recorded already,
// with the status its code maps to (see failureCoder and statusOf).
func (c *call) fail(err error) error {
	return statusOf(failureCoder(err), err)
}

// failService answers a delegated service's error the way the HTTP handler
// does: logged and recorded on the span, then the status its code maps to
// (see serviceErrorCoder and statusOf).
func (c *call) failService(err error) error {
	c.log.Errorz("service operation failed", zap.Error(err))
	gstotel.RecordError(c.span, err)
	return statusOf(serviceErrorCoder(err), err)
}

// statusOf returns the status a call answers err with, the failure of a flow
// or a service, from the code the HTTP listener would answer it with: the
// code's mapping (see grpcserver.StatusOfCoder) for every failure the
// listener recognizes — a refusal, a missing record, a conflict, a service
// error with a status of its own — and Internal with a fixed message for the
// generic failure, the one the listener answers a failure it did not
// recognize with, an unknown database error or a plain error of a hook or
// service: a failure of the server's own is the server's, not, as the HTTP
// envelope has it, the client's, and its text stays out of the answer the
// way grpcserver.StatusError keeps it out.
func statusOf(coder types.Coder, err error) error {
	if coder == CodeFailure {
		return grpcserver.StatusError(err)
	}
	return grpcserver.StatusOfCoder(coder)
}

// finish ends a call whose flow or service returned, checking the service
// contexts it built: a service that asked one of them for a raw HTTP
// response — a body, a stream, a cookie — believes it answered, and the call
// cannot carry what it wrote, so the call answers Internal and logs why (gg
// check reports the call at generation time; this is the transport's own
// refusal). nil otherwise.
func (c *call) finish() error {
	if !slices.ContainsFunc(c.built, types.RawResponseAttempted) {
		return nil
	}
	err := errors.New("the service wrote an HTTP response the call cannot carry: an action served over gRPC must not call Data, SSE or SetCookie")
	c.log.Errorz("service operation failed", zap.Error(err))
	gstotel.RecordError(c.span, err)
	return grpcserver.StatusError(err)
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
// delegation the factories make when M, REQ and RSP differ. Given the route
// parameters, the query of a List or Get, and the payload the request
// message decoded into, the call validates the payload the way the handler
// validates a bound body — not for a List or Get, whose HTTP request binds
// no body — runs the method in its service span on a service context
// answering the parameters, the query and the caller, and answers with its
// result, or with the status its error maps to (see statusOf). phase must be
// one gRPC serves: the HTTP-only actions have no rpc, so their phase panics
// here, as the route registers.
func ServiceCall[M types.Model, REQ types.Request, RSP types.Response](phase consts.Phase, route string) func(ctx context.Context, params map[string]string, query Query, req REQ) (RSP, error) {
	invoke := serviceMethod[M, REQ, RSP](phase)
	meta := newFactoryMeta[M, REQ, RSP](route, phase)
	binds := phase != consts.PHASE_LIST && phase != consts.PHASE_GET
	return func(ctx context.Context, params map[string]string, query Query, req REQ) (RSP, error) {
		var zero RSP
		c, err := meta.beginQueryCall(ctx, params, query)
		defer c.end()
		if err != nil {
			return zero, c.invalid(err)
		}
		meta.normalizeRequest(&req)
		if binds {
			if err = validateRequest(req); err != nil {
				return zero, c.invalid(err)
			}
		}
		rsp, err := meta.traceServiceOperation(c.ctx, phase, func(spanCtx context.Context) (RSP, error) {
			return invoke(meta.service(), c.serviceContext(spanCtx, phase), req)
		})
		if err != nil {
			return zero, c.failService(err)
		}
		return answer(c, rsp)
	}
}

// serviceMethod returns the method of a service serving phase, the one the
// HTTP handler of the phase delegates to when M, REQ and RSP differ. A phase
// gRPC does not serve — the HTTP-only actions Import, Export and SSE, and the
// hook phases — has no rpc to be called from, so it panics.
func serviceMethod[M types.Model, REQ types.Request, RSP types.Response](phase consts.Phase) func(svc types.Service[M, REQ, RSP], sc *types.ServiceContext, req REQ) (RSP, error) {
	switch phase {
	case consts.PHASE_CREATE:
		return types.Service[M, REQ, RSP].Create
	case consts.PHASE_DELETE:
		return types.Service[M, REQ, RSP].Delete
	case consts.PHASE_UPDATE:
		return types.Service[M, REQ, RSP].Update
	case consts.PHASE_PATCH:
		return types.Service[M, REQ, RSP].Patch
	case consts.PHASE_LIST:
		return types.Service[M, REQ, RSP].List
	case consts.PHASE_GET:
		return types.Service[M, REQ, RSP].Get
	case consts.PHASE_CREATE_MANY:
		return types.Service[M, REQ, RSP].CreateMany
	case consts.PHASE_DELETE_MANY:
		return types.Service[M, REQ, RSP].DeleteMany
	case consts.PHASE_UPDATE_MANY:
		return types.Service[M, REQ, RSP].UpdateMany
	case consts.PHASE_PATCH_MANY:
		return types.Service[M, REQ, RSP].PatchMany
	}
	panic(fmt.Sprintf("controller: phase %q has no rpc; ServiceCall serves the actions of a model's gRPC service", phase))
}
