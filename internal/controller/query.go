package controller

import (
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/internal/urlquery"
	"github.com/stoewer/go-strcase"
)

// listQueryKeys are List controller parameters that belong to model.Query
// rather than to the resource model's own filter fields.
var listQueryKeys = map[string]struct{}{
	consts.QUERY_EXPAND:  {},
	consts.QUERY_DEPTH:   {},
	consts.QUERY_SORT_BY: {},
}

// listPageQueryKey is enabled by model.Pagination only: offset paging
// conflicts with cursor semantics, so cursor-only models reject it.
var listPageQueryKey = map[string]struct{}{
	consts.QUERY_PAGE: {},
}

// listSizeQueryKey is enabled by model.Pagination or model.Cursor: both
// paging styles need a client-adjustable page/batch size.
var listSizeQueryKey = map[string]struct{}{
	consts.QUERY_SIZE: {},
}

// listCursorQueryKeys are enabled by model.Cursor. A model may opt in to both
// cursor and sort parameters, but a single request may not use both at once:
// the cursor column and travel direction define the stable order, which
// checkCursorOrderConflict enforces.
var listCursorQueryKeys = map[string]struct{}{
	consts.QUERY_CURSOR_VALUE: {},
	consts.QUERY_CURSOR_FIELD: {},
	consts.QUERY_CURSOR_NEXT:  {},
}

// exportLimitQueryKey is the export's cap on the records it lists, which no
// List reads: a List pages by _size.
var exportLimitQueryKey = map[string]struct{}{
	consts.QUERY_LIMIT: {},
}

// decodeListQuery rejects the framework query keys the model has not opted in
// to via model.Query, model.Pagination, or model.Cursor, and the export's
// _limit, then decodes the remaining URL query parameters into the model's
// own query fields. The explicit gate keeps the rejection uniform: it reports
// every such key in one error, including the two urlquery.Decode would
// silently drop, _size for a model that cannot carry it and _limit, which it
// lets through for the export.
func decodeListQuery[M types.Model](m M, query map[string][]string) error {
	rejected := matchQueryKeys(query, exportLimitQueryKey)
	if !modelregistry.IsQueryable(m) {
		rejected = append(rejected, matchQueryKeys(query, listQueryKeys)...)
	}
	paginatable := modelregistry.IsPaginatable(m)
	cursorable := modelregistry.IsCursorable(m)
	if !paginatable {
		rejected = append(rejected, matchQueryKeys(query, listPageQueryKey)...)
	}
	if !paginatable && !cursorable {
		rejected = append(rejected, matchQueryKeys(query, listSizeQueryKey)...)
	}
	if !cursorable {
		rejected = append(rejected, matchQueryKeys(query, listCursorQueryKeys)...)
	}
	if len(rejected) > 0 {
		return urlquery.UnsupportedParameterError(rejected)
	}
	return urlquery.Decode(query, m)
}

// matchQueryKeys returns the query keys present in the given key set.
func matchQueryKeys(query map[string][]string, keys map[string]struct{}) []string {
	matched := make([]string, 0)
	for key := range query {
		if _, found := keys[key]; found {
			matched = append(matched, key)
		}
	}
	return matched
}

// checkCursorOrderConflict reports the client error for combining cursor
// pagination with an explicit sort order. Cursor pagination derives its
// ORDER BY from the cursor column, so a second order source would demote that
// column to a secondary sort key and invalidate the boundary condition the
// cursor relies on, which shows up as pages that skip or repeat rows.
func checkCursorOrderConflict(cursor types.Cursor, orders []types.Order) error {
	if cursor.Enabled() && len(orders) > 0 {
		return errors.New("cursor pagination defines its own ordering: _sort_by cannot be combined with _cursor_value")
	}
	return nil
}

// maxExpandDepth is the largest _depth a request may ask for; a value
// outside 1 to maxExpandDepth falls back to 1 (see parseExpandQuery). Every
// depth level becomes one more recursive preload query, so the cap keeps a
// single request from fanning out unbounded database work.
const maxExpandDepth = 10

// parseExpandQuery resolves the _expand and _depth parameters of the parsed
// query against the model's expandable association paths. Expand names are
// matched against m.Expands() ignoring case and snake case punctuation, so
// "childItems" and "child_items" both select "ChildItems"; "_expand=all"
// selects every expandable path. _depth (default 1; a value outside 1 to
// maxExpandDepth falls back to 1) repeats the to-many associations holding
// the model itself for recursive preloading (see
// modelschema.RecursiveAssociations), e.g. expand "Children" with depth 3
// becomes "Children.Children.Children"; every other association ignores
// depth, its rows having no such association to descend into.
func parseExpandQuery(query url.Values, m types.Model) []string {
	if !query.Has(consts.QUERY_EXPAND) {
		return nil
	}
	depth := 1
	if query.Has(consts.QUERY_DEPTH) {
		depth, _ = strconv.Atoi(query.Get(consts.QUERY_DEPTH))
		if depth < 1 || depth > maxExpandDepth {
			depth = 1
		}
	}

	items := strings.Split(query.Get(consts.QUERY_EXPAND), ",")
	if len(items) > 0 && items[0] == consts.VALUE_ALL { // expand all fields
		items = m.Expands()
	}
	var matched []string
	for _, e := range m.Expands() {
		for _, item := range items {
			if strings.EqualFold(strcase.SnakeCase(item), strcase.SnakeCase(e)) {
				matched = append(matched, e)
			}
		}
	}

	recursive := modelschema.RecursiveAssociations(reflect.TypeOf(m))
	var expands []string
	for _, e := range matched {
		if _, ok := recursive[e]; !ok {
			expands = append(expands, e)
			continue
		}
		t := make([]string, depth)
		for i := range depth {
			t[i] = e
		}
		// If expand="Children" and depth=3, the depth expanded is
		// "Children.Children.Children".
		expands = append(expands, strings.Join(t, "."))
	}
	return expands
}
