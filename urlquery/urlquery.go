// Package urlquery turns URL query parameters into database query arguments.
//
// It exists for services that implement a list action themselves. A model
// whose List action declares its own Result type takes over the whole request,
// so the framework list controller no longer parses the query for it. Instead
// of reimplementing the parsing, such a service builds its query from the same
// functions the controller uses, and its endpoint keeps behaving exactly like
// every framework-driven list.
//
// Each function maps to one database query argument:
//
//	Decode        -> the query value Database.WithQuery takes
//	Filters       -> QueryOptions.Filters, from "field[op]=value" parameters
//	PresentFields -> QueryOptions.PresentFields, so "enabled=false" filters
//	Pagination    -> Database.WithPagination
//	Cursor        -> Database.WithCursor
//
// Which parameters a request may use is decided by the model: capabilities are
// opted in to by embedding model.Query, model.Pagination or model.Cursor, and
// a parameter the model did not opt in to is ignored rather than rejected.
// Malformed filters are the one exception and always fail, because silently
// dropping a mistyped filter would widen the result set.
//
// A typical list service builds its query like this:
//
//	query := new(Sample)
//	if err := urlquery.Decode(ctx.Query(), query); err != nil {
//	    return nil, service.NewError(http.StatusBadRequest, err.Error())
//	}
//	filters, err := urlquery.Filters(ctx.Query(), query)
//	if err != nil {
//	    return nil, service.NewError(http.StatusBadRequest, err.Error())
//	}
//	items := make([]*Sample, 0)
//	err = database.Database[*Sample](ctx).
//	    WithQuery(query, types.QueryOptions{
//	        AllowEmpty:    true,
//	        PresentFields: urlquery.PresentFields(ctx.Query()),
//	        Filters:       filters,
//	    }).
//	    WithCursor(urlquery.Cursor(ctx.Query(), query)).
//	    WithPagination(urlquery.Pagination(ctx.Query(), query)).
//	    List(&items)
//
// A service that also reports a total must count with the very same query
// value and options, otherwise the total and the page disagree.
//
// Framework code imports internal/urlquery directly.
package urlquery

import (
	"net/url"

	"github.com/hydroan/gst/internal/urlquery"
	"github.com/hydroan/gst/types"
)

// Decode fills the model's own query fields from a request, producing the
// query value Database.WithQuery takes as its first argument. The keys Filters
// owns never reach the decoder, and neither do framework parameters the model
// cannot carry; every other key must map to a field of the model, so a
// mistyped filter name is reported instead of silently widening the result set.
func Decode(q url.Values, m types.Model) error {
	return urlquery.Decode(q, m)
}

// Filters extracts the field-level operator filters ("field[op]=value") of a
// request, plus the bare framework timestamp keys ("created_at",
// "updated_at"), which act as exact-match filters. The field must resolve to a
// filterable column of the model and the operator must be known; anything else
// is rejected so a mistyped filter can never silently widen the result set.
// Filters require the model to embed model.Query.
func Filters(q url.Values, m types.Model) ([]types.Filter, error) {
	return urlquery.Filters(q, m)
}

// PresentFields collects the model filter keys the request provided
// explicitly, so the database layer keeps their zero values (false, 0) as real
// conditions instead of dropping them as unset.
func PresentFields(q url.Values) map[string]struct{} {
	return urlquery.PresentFields(q)
}

// Pagination returns the page and size arguments of a request, ready to be
// passed to Database.WithPagination. Models embedding model.Pagination take
// both from the request and models embedding model.Cursor take the size only;
// any other model keeps the framework's full-table safety limit.
func Pagination(q url.Values, m types.Model) (page, size int) {
	return urlquery.Pagination(q, m)
}

// Cursor returns the cursor value, direction and field of a request, ready to
// be passed to Database.WithCursor. Models that did not embed model.Cursor
// yield a zero cursor, which WithCursor treats as a no-op.
func Cursor(q url.Values, m types.Model) (value string, next bool, field string) {
	return urlquery.Cursor(q, m)
}
