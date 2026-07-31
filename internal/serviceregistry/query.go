package serviceregistry

import (
	"reflect"

	"github.com/hydroan/gst/internal/urlquery"
	"github.com/hydroan/gst/types"
)

// The Query methods below translate the request's URL query parameters into
// the arguments a database query is built from, so a service that implements a
// list action itself parses the request exactly like the framework list
// controller does.
//
// A model whose List action declares its own Result type takes over the whole
// request, and the controller no longer parses the query for it. Rather than
// reimplementing the parsing, such a service asks its own base for the parsed
// arguments; the model they are parsed against is the base's own M, so a
// service can never accidentally parse a request against the wrong model.
//
// Each method maps to one database query argument:
//
//	QueryDecode        -> the query value Database.WithQuery takes
//	QueryFilters       -> QueryOptions.Filters, from "field[op]=value" parameters
//	QueryPresentFields -> QueryOptions.PresentFields, so "enabled=false" filters
//	QueryOrders        -> Database.WithOrder
//	QueryPagination    -> Database.WithPagination
//	QueryCursor        -> Database.WithCursor
//
// Which parameters a request may use is decided by the model: capabilities are
// opted in to by embedding model.Query, model.Pagination or model.Cursor, and
// a parameter the model did not opt in to is ignored rather than rejected.
// Malformed filters are the one exception and always fail, because silently
// dropping a mistyped filter would widen the result set.
//
// A typical list service builds its query like this:
//
//	query, err := s.QueryDecode(ctx)
//	if err != nil {
//	    return nil, service.NewError(http.StatusBadRequest, err.Error())
//	}
//	query.TenantID = tenant // mandatory scoping the client cannot influence
//
//	filters, err := s.QueryFilters(ctx)
//	if err != nil {
//	    return nil, service.NewError(http.StatusBadRequest, err.Error())
//	}
//	opts := types.QueryOptions{
//	    AllowEmpty:    true,
//	    PresentFields: s.QueryPresentFields(ctx),
//	    Filters:       filters,
//	}
//
//	cursor, err := s.QueryCursor(ctx)
//	if err != nil {
//	    return nil, service.NewError(http.StatusBadRequest, err.Error())
//	}
//	orders, err := s.QueryOrders(ctx)
//	if err != nil {
//	    return nil, service.NewError(http.StatusBadRequest, err.Error())
//	}
//	err = database.Database[*Sample](ctx).
//	    WithQuery(query, opts).
//	    WithCursor(cursor).
//	    WithOrder(orders...).
//	    WithPagination(s.QueryPagination(ctx)).
//	    List(&items)
//
// A service that also reports a total must count with the very same query
// value and options, otherwise the total and the page disagree.

// QueryDecode returns a new model with its own query fields filled from the
// request, ready to be passed to Database.WithQuery.
//
// The keys QueryFilters owns never reach the decoder, and neither do framework
// parameters the model cannot carry; every other key must map to a field of
// the model, so a mistyped filter name is reported instead of silently
// widening the result set. The model is returned even when decoding fails, so
// a caller that tolerates partial input can still use it.
func (Base[M, REQ, RSP]) QueryDecode(ctx *types.ServiceContext) (M, error) {
	m := reflect.New(reflect.TypeFor[M]().Elem()).Interface().(M) //nolint:errcheck
	return m, urlquery.Decode(ctx.Query(), m)
}

// QueryFilters returns the field-level operator filters ("field[op]=value") of
// the request, plus the bare framework timestamp keys ("created_at",
// "updated_at"), which act as exact-match filters. The field must resolve to a
// filterable column of the model and the operator must be known; anything else
// is rejected so a mistyped filter can never silently widen the result set.
// Filters require the model to embed model.Query.
func (Base[M, REQ, RSP]) QueryFilters(ctx *types.ServiceContext) ([]types.Filter, error) {
	return urlquery.Filters(ctx.Query(), zeroModel[M]())
}

// QueryPresentFields returns the model filter keys the request provided
// explicitly, so the database layer keeps their zero values (false, 0) as real
// conditions instead of dropping them as unset.
func (Base[M, REQ, RSP]) QueryPresentFields(ctx *types.ServiceContext) map[string]struct{} {
	return urlquery.PresentFields(ctx.Query())
}

// QueryOrders returns the ORDER BY terms of the request, ready to be passed to
// Database.WithOrder. Models embedding model.Query take them from _sort_by;
// any other model yields no order. An unknown column or direction is a client
// error and is reported as such.
func (Base[M, REQ, RSP]) QueryOrders(ctx *types.ServiceContext) ([]types.Order, error) {
	return urlquery.Orders(ctx.Query(), zeroModel[M]())
}

// QueryPagination returns the page and size arguments of the request, ready to
// be passed to Database.WithPagination. Models embedding model.Pagination take
// both from the request and models embedding model.Cursor take the size only;
// any other model keeps the framework's full-table safety limit.
func (Base[M, REQ, RSP]) QueryPagination(ctx *types.ServiceContext) (page, size int) {
	return urlquery.Pagination(ctx.Query(), zeroModel[M]())
}

// QueryCursor returns the cursor position of the request, ready to be passed
// to Database.WithCursor. Models that did not embed model.Cursor yield a zero
// cursor, which WithCursor treats as a no-op. An unknown cursor column, or a
// cursor value the column's Go type cannot represent, is a client error and
// is reported as such.
func (Base[M, REQ, RSP]) QueryCursor(ctx *types.ServiceContext) (types.Cursor, error) {
	return urlquery.Cursor(ctx.Query(), zeroModel[M]())
}

// zeroModel returns the typed nil of M. The parsers only read the model's type
// to assert its query capabilities and resolve its columns, so allocating one
// would be wasted work.
func zeroModel[M types.Model]() M {
	var m M
	return m
}
