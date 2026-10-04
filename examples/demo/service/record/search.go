package record

import (
	"net/http"

	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/database"
	gstmodel "github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
)

// Search answers GET /api/records/search, a custom List that takes the whole
// request over: the framework parses no query parameter for it, so the
// service reads them back through the query helpers of service.Base, and
// the filters, sorting and pagination behave exactly as on the framework's
// own lists.
type Search struct {
	service.Base[*model.Record, *gstmodel.Empty, *model.RecordSearchRsp]
}

// List lists the caller's records the query parameters select, a page of
// them with the count of all that match: type=text is a field of the model,
// type[in]=text,image or title[startswith]=draft a filter, _sort_by=title
// desc an order, _page and _size the page. Cursor pagination, _cursor_field
// and _cursor_value, is read with QueryCursor and passed to WithCursor in
// place of the page.
func (s *Search) List(ctx *gst.ServiceContext, _ *gstmodel.Empty) (*model.RecordSearchRsp, error) {
	query, err := s.QueryModel(ctx)
	if err != nil {
		return nil, gst.NewError(http.StatusBadRequest, err.Error())
	}
	// A condition the client cannot lift: the caller's own records.
	query.UserID = ctx.UserID()
	filters, err := s.QueryFilters(ctx)
	if err != nil {
		return nil, gst.NewError(http.StatusBadRequest, err.Error())
	}
	opts := gst.QueryOptions{AllowEmpty: true, PresentFields: s.QueryPresentFields(ctx), Filters: filters}
	orders, err := s.QueryOrders(ctx)
	if err != nil {
		return nil, gst.NewError(http.StatusBadRequest, err.Error())
	}
	page, size := s.QueryPagination(ctx)

	rsp := &model.RecordSearchRsp{Items: make([]*model.Record, 0, size)}
	if err := database.Database[*model.Record](ctx).
		WithQuery(query, opts).
		WithOrder(orders...).
		WithPagination(page, size).
		List(&rsp.Items); err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "failed to list the records", err)
	}
	// The count takes the same query and options, so the total matches the page.
	if err := database.Database[*model.Record](ctx).WithQuery(query, opts).Count(&rsp.Total); err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "failed to count the records", err)
	}
	return rsp, nil
}
