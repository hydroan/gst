package controller

import (
	"context"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/apiaction"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/internal/urlquery"
	"github.com/hydroan/gst/logger"
	"go.uber.org/zap"
)

// listFlow runs the list flow: it decodes the query parameters the request
// carries into the model's own query fields and the framework's pagination,
// cursor, expansion, depth, ordering and field operator filters, lets the
// service scope the query in its Filter hook, runs the list hooks around the
// read, and records the operation. The total counts the rows the query
// matches; under cursor pagination, which provides none, it is 0.
func (a *action[M, REQ, RSP]) listFlow(ctx context.Context, newServiceContext serviceContextFunc) ([]M, int, error) {
	log := logger.Controller.WithContext(ctx, apiaction.List)
	svc := a.service()

	// The request's memoized query parse, shared by every parser below; the
	// parsers only read the values.
	query := requestctx.QueryValues(ctx)

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

	expands, err := parseExpandQuery(query, m)
	if err != nil {
		return nil, 0, failWith(ctx, log, "parse query parameter failed", err, invalidArgument(err))
	}

	data := make([]M, 0)

	// 1.Perform business logic processing before list resources.
	if err = a.traceServiceHook(ctx, apiaction.ListBefore, svc, newServiceContext, func(sc *types.ServiceContext) error {
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
	filterCtx := newServiceContext(ctx, apiaction.List)
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
	if err = a.traceServiceHook(ctx, apiaction.ListAfter, svc, newServiceContext, func(sc *types.ServiceContext) error {
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
	if err = audit.RecordOperation(ctx, m, types.OperationList,
		func() *modellogmgmt.OperationLog {
			return operationLog(ctx, a.name)
		}); err != nil {
		log.Warnz("record operation log failed", zap.Error(err))
	}

	return data, total, nil
}
