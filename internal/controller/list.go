package controller

import (
	"context"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/logger"
	modellogmgmt "github.com/hydroan/gst/internal/modulemodel/logmgmt"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/urlquery"
	"go.uber.org/zap"
)

// listFlow runs the list flow: it decodes the query parameters the request
// carries into the model's own query fields and the framework's pagination,
// cursor, expansion, depth, ordering and field operator filters, lets the
// service scope the query in its Filter hook, runs the list hooks around the
// read, and records the operation. The total counts the rows the query
// matches; under cursor pagination, which provides none, it is 0. The
// boolean tells whether records lie past the page answered: by the total
// where one is counted, and under cursor pagination by one record read past
// the page, in the direction the request travels, which is dropped before the
// service's ListAfter hook.
func (a *action[M, REQ, RSP]) listFlow(ctx context.Context, newServiceContext serviceContextFunc) ([]M, int, bool, error) {
	log := logger.Controller.WithContext(ctx, apiaction.List)
	svc := a.service()

	// The request's memoized query parse, shared by every parser below; the
	// parsers only read the values.
	query := requestctx.QueryValues(ctx)

	m := a.newModel()

	if err := decodeListQuery(m, query); err != nil {
		return nil, 0, false, failWith(ctx, log, "parse query parameter failed", err, invalidArgument(err))
	}
	// The cursor is read ahead of the filters and the order: a feed refuses
	// a page or an order whole, whatever they or the rest of the query hold.
	cursor, err := urlquery.Cursor(query, m)
	if err != nil {
		return nil, 0, false, failWith(ctx, log, "parse query parameter failed", err, invalidArgument(err))
	}
	filters, err := urlquery.Filters(query, m)
	if err != nil {
		return nil, 0, false, failWith(ctx, log, "parse query parameter failed", err, invalidArgument(err))
	}
	present := urlquery.PresentFields(query)

	orders, err := urlquery.Orders(query, m)
	if err != nil {
		return nil, 0, false, failWith(ctx, log, "parse query parameter failed", err, invalidArgument(err))
	}

	expands, err := parseExpandQuery(query, m)
	if err != nil {
		return nil, 0, false, failWith(ctx, log, "parse query parameter failed", err, invalidArgument(err))
	}

	data := make([]M, 0)

	// 1.Perform business logic processing before list resources.
	if err = a.traceServiceHook(ctx, apiaction.ListBefore, svc, newServiceContext, func(sc *gst.ServiceContext) error {
		return svc.ListBefore(sc, &data)
	}); err != nil {
		return nil, 0, false, failService(ctx, log, err)
	}
	// 2.Let the service rewrite the query condition and options; the typical
	// use is row-level data scoping. Filter runs once and the result is
	// shared by List and Count below, so both see the same condition set.
	queryOpts := gst.QueryOptions{
		AllowEmpty:    true,
		PresentFields: present,
		Filters:       filters,
	}
	filterCtx := newServiceContext(ctx, apiaction.List)
	if m, queryOpts, err = svc.Filter(filterCtx, m, queryOpts); err != nil {
		return nil, 0, false, failService(ctx, log, err)
	}
	if err = httpOnlyMethodCalled(filterCtx); err != nil {
		return nil, 0, false, failService(ctx, log, err)
	}
	// 3.List resources from database. A feed counts nothing (see below), so
	// it reads one record past the page to tell whether records lie past it:
	// no second query, and no empty page for the client to ask for.
	page, size := urlquery.Pagination(query)
	limit := size
	if cursor.Enabled() {
		limit++
	}
	if err = database.Database[M](ctx).
		WithPagination(page, limit).
		WithQuery(m, queryOpts).
		WithCursor(cursor).
		WithExpand(expands, orders...).
		WithOrder(orders...).
		List(&data); err != nil {
		return nil, 0, false, failDatabase(ctx, log, err)
	}
	var hasMore bool
	if cursor.Enabled() && len(data) > size {
		// The record past the page is the one farthest along the request's
		// direction: the last one, or the first one of a page read backward,
		// which the database hands back in the feed's own order.
		hasMore = true
		if cursor.Backward() {
			data = data[1:]
		} else {
			data = data[:size]
		}
	}
	// 4.Perform business logic processing after list resources.
	if err = a.traceServiceHook(ctx, apiaction.ListAfter, svc, newServiceContext, func(sc *gst.ServiceContext) error {
		return svc.ListAfter(sc, &data)
	}); err != nil {
		return nil, 0, false, failService(ctx, log, err)
	}
	var total int
	// The count is skipped under cursor pagination, the first page included,
	// so the total answered stays 0: a feed is read page by page at constant
	// cost, and a count of it would be stale by the next page.
	if !cursor.Enabled() {
		if err = database.Database[M](ctx).
			WithQuery(m, queryOpts).
			Count(&total); err != nil {
			return nil, 0, false, failDatabase(ctx, log, err)
		}
		// Records lie past the page while the total exceeds the page*size
		// records the pages up to this one hold; the division tells the same
		// without a product a page number of any size could overflow.
		hasMore = (total-1)/size >= page
	}

	// 5.record operation log to database.
	if err = audit.RecordOperation(ctx, m, gst.OperationList,
		func() *modellogmgmt.OperationLog {
			return operationLog(ctx, a.name)
		}); err != nil {
		log.Warnz("record operation log failed", zap.Error(err))
	}

	return data, total, hasMore, nil
}
