package controller

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/consts"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/internal/urlquery"
	"github.com/hydroan/gst/logger"
	"go.uber.org/zap"
)

// ListHandler returns a Gin handler that lists resources.
//
// When M, REQ, and RSP are the same type, the handler runs the list flow (see
// listFlow) and returns the items with a total count, zero under cursor
// pagination, which counts nothing.
//
// The automatic listing branch supports model schema fields plus framework query
// parameters for pagination, cursor pagination, expansion, depth, ordering, and
// field operator filters.
//
// When REQ or RSP differs from M, the handler is the phase service's (see
// serviceHandler): its List method runs on a zero-value REQ, the GET request
// carrying no body; the service reads ServiceContext.Query().
func ListHandler[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	a := newAction[M, REQ, RSP](routeFromConfig(cfg...), consts.List, consts.ListBefore, consts.ListAfter)
	if !a.typesEqual {
		return a.serviceHandler()
	}
	return func(c *gin.Context) {
		// The span context lives on in the request context the span start
		// rebinds, which requestContext reads for the flow.
		_, span := a.startControllerSpan(c)
		defer span.End()

		items, total, err := a.listFlow(requestContext(c), ginServiceContext(c))
		if err != nil {
			response.Error(c, err)
			return
		}
		response.JSON(c, gin.H{
			"items": items,
			"total": total,
		})
	}
}

// ListCall returns the list call of M on route, the counterpart of the
// handler ListHandler returns for the generated handler of a List rpc: given
// the route parameters and the query the request message carries, it runs
// the list flow (see listFlow) on the query rendered as the HTTP one (see
// Query) and answers with the items and the total, or with the status the
// failure maps to (see call).
func ListCall[M types.Model](route string) func(ctx context.Context, params map[string]string, query Query) ([]M, int, error) {
	a := newAction[M, M, M](route, consts.List, consts.ListBefore, consts.ListAfter)
	return func(ctx context.Context, params map[string]string, query Query) ([]M, int, error) {
		c, err := a.beginQueryCall(ctx, params, query)
		defer c.end()
		if err != nil {
			return nil, 0, c.invalid(err)
		}
		items, total, err := a.listFlow(c.ctx, c.serviceContext)
		if err != nil {
			return nil, 0, c.fail(err)
		}
		if err := c.finish(); err != nil {
			return nil, 0, err
		}
		return items, total, nil
	}
}

// listFlow runs the list flow: it decodes the query parameters the request
// carries into the model's own query fields and the framework's pagination,
// cursor, expansion, depth, ordering and field operator filters, lets the
// service scope the query in its Filter hook, runs the list hooks around the
// read, and records the operation. The total counts the rows the query
// matches; under cursor pagination, which provides none, it is 0.
func (a *action[M, REQ, RSP]) listFlow(ctx context.Context, newServiceContext serviceContextFunc) ([]M, int, error) {
	log := logger.Controller.WithContext(ctx, consts.List)
	svc := a.service()

	// The request's memoized query parse, shared by every parser below; the
	// parsers only read the values.
	query := requestctx.QueryValues(ctx)

	// 'm' is a fresh model instance, such as: &model.User{ID: myid, Name: myname}.
	m := a.newModel()

	if err := decodeListQuery(m, query); err != nil {
		return nil, 0, failWith(ctx, log, "parse query parameter failed", err, invalidArgument(err))
	}
	filters, err := urlquery.Filters(query, m)
	if err != nil {
		return nil, 0, failWith(ctx, log, "parse query parameter failed", err, invalidArgument(err))
	}
	present := urlquery.PresentFields(query)

	orders, err := urlquery.Orders(query, m)
	if err != nil {
		return nil, 0, failWith(ctx, log, "parse query parameter failed", err, invalidArgument(err))
	}

	cursor, err := urlquery.Cursor(query, m)
	if err != nil {
		return nil, 0, failWith(ctx, log, "parse query parameter failed", err, invalidArgument(err))
	}

	if err = checkCursorOrderConflict(cursor, orders); err != nil {
		return nil, 0, failWith(ctx, log, "parse query parameter failed", err, invalidArgument(err))
	}

	data := make([]M, 0)
	expands := parseExpandQuery(query, m)

	// 1.Perform business logic processing before list resources.
	if err = a.traceServiceHook(ctx, consts.ListBefore, svc, newServiceContext, func(sc *types.ServiceContext) error {
		return svc.ListBefore(sc, &data)
	}); err != nil {
		return nil, 0, failService(ctx, log, err)
	}
	// 2.Let the service rewrite the query condition and options; the typical
	// use is row-level data scoping. Filter runs once and the result is
	// shared by List and Count below, so both see the same condition set.
	queryOpts := types.QueryOptions{
		AllowEmpty:    true,
		PresentFields: present,
		Filters:       filters,
	}
	filterCtx := newServiceContext(ctx, consts.List)
	if m, queryOpts, err = svc.Filter(filterCtx, m, queryOpts); err != nil {
		return nil, 0, failService(ctx, log, err)
	}
	if err = httpOnlyMethodCalled(filterCtx); err != nil {
		return nil, 0, failService(ctx, log, err)
	}
	// 3.List resources from database.
	if err = database.Database[M](ctx).
		WithPagination(urlquery.Pagination(query, m)).
		WithQuery(m, queryOpts).
		WithCursor(cursor).
		WithExpand(expands, orders...).
		WithOrder(orders...).
		List(&data); err != nil {
		return nil, 0, failDatabase(ctx, log, err)
	}
	// 4.Perform business logic processing after list resources.
	if err = a.traceServiceHook(ctx, consts.ListAfter, svc, newServiceContext, func(sc *types.ServiceContext) error {
		return svc.ListAfter(sc, &data)
	}); err != nil {
		return nil, 0, failService(ctx, log, err)
	}
	var total int
	// The count is skipped under cursor pagination, the first page included,
	// so the total answered stays 0: a feed is read page by page at constant
	// cost, and a count of it would be stale by the next page.
	if !cursor.Enabled() {
		if err = database.Database[M](ctx).
			WithQuery(m, queryOpts).
			Count(&total); err != nil {
			return nil, 0, failDatabase(ctx, log, err)
		}
	}

	// 5.record operation log to database.
	if err = audit.RecordOperation(ctx, m, consts.OP_LIST,
		func() *modellogmgmt.OperationLog {
			return operationLog(ctx, a.name)
		}); err != nil {
		log.Warnz("record operation log failed", zap.Error(err))
	}

	return data, total, nil
}
