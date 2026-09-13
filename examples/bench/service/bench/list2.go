package bench

import (
	"net/http"

	"bench/model/bench"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
)

type List2 struct {
	service.Base[*bench.Bench, *model.Empty, *bench.ListRsp]
}

// List replays the standard framework List action's full query chain in dry
// run end to end, producing no database I/O: query parsing (model conditions,
// operator filters, ordering, cursor, present fields, pagination) and
// condition building (WithQuery model and filter parameters, SELECT and COUNT
// generation) all run for real; only the final execution is skipped. It
// benchmarks the framework's query machinery free of database cost.
func (l *List2) List(ctx *gst.ServiceContext, req *model.Empty) (rsp *bench.ListRsp, err error) {
	var m *bench.Bench
	var filters []gst.Filter
	var orders []gst.Order
	var cursor gst.Cursor

	if m, err = l.QueryModel(ctx); err != nil {
		return nil, service.NewError(http.StatusBadRequest, err.Error())
	}
	if filters, err = l.QueryFilters(ctx); err != nil {
		return nil, service.NewError(http.StatusBadRequest, err.Error())
	}
	if orders, err = l.QueryOrders(ctx); err != nil {
		return nil, service.NewError(http.StatusBadRequest, err.Error())
	}
	if cursor, err = l.QueryCursor(ctx); err != nil {
		return nil, service.NewError(http.StatusBadRequest, err.Error())
	}
	page, size := l.QueryPagination(ctx)

	opts := gst.QueryOptions{
		AllowEmpty:    true,
		PresentFields: l.QueryPresentFields(ctx),
		Filters:       filters,
	}
	items := make([]*bench.Bench, 0)
	if err = database.Database[*bench.Bench](ctx).
		WithDryRun().
		WithPagination(page, size).
		WithQuery(m, opts).
		WithCursor(cursor).
		WithOrder(orders...).
		List(&items); err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "build list query failed", err)
	}
	// Mirror the standard List action: the total reuses the same query conditions through a separately built COUNT.
	count := new(int)
	if err = database.Database[*bench.Bench](ctx).
		WithDryRun().
		WithQuery(m, opts).
		Count(count); err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "build count query failed", err)
	}

	return &bench.ListRsp{Msg: "hi list2"}, nil
}
