package controller

import (
	"net/url"
	"reflect"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/internal/urlquery"
	"github.com/stoewer/go-strcase"
)

// expandAll is the value of the expand parameter asking for every
// association.
const expandAll = "all"

// listQueryKeys are List controller parameters that belong to model.Query
// rather than to the resource model's own filter fields.
var listQueryKeys = map[string]struct{}{
	types.QueryExpand: {},
	types.QueryDepth:  {},
	types.QuerySortBy: {},
}

// listPageQueryKey is enabled by model.Pagination only: offset paging
// conflicts with cursor semantics, so cursor-only models reject it.
var listPageQueryKey = map[string]struct{}{
	types.QueryPage: {},
}

// listSizeQueryKey is enabled by model.Pagination or model.Cursor: both
// paging styles need a client-adjustable page/batch size.
var listSizeQueryKey = map[string]struct{}{
	types.QuerySize: {},
}

// listCursorQueryKeys are enabled by model.Cursor. A model may opt in to both
// cursor and sort parameters, but a single request may not use both at once:
// the cursor column and travel direction define the stable order, which
// checkCursorOrderConflict enforces.
var listCursorQueryKeys = map[string]struct{}{
	types.QueryCursorValue: {},
	types.QueryCursorField: {},
	types.QueryCursorNext:  {},
}

// decodeListQuery rejects the framework query keys the model has not opted in
// to via model.Query, model.Pagination, or model.Cursor, then decodes the
// remaining URL query parameters into the model's own query fields. The
// explicit gate keeps the rejection uniform: it reports every non-opted
// capability key in one error, including _size, which urlquery.Decode would
// silently drop for a model that cannot carry it.
func decodeListQuery[M types.Model](m M, query map[string][]string) error {
	rejected := make([]string, 0)
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

// parseExpandQuery resolves the _expand and _depth parameters of the parsed
// query against the model's expandable association paths, which come back
// in the order m.Expands() lists them. Expand names are matched against
// m.Expands() ignoring case, snake case punctuation and the spaces around
// them, so "childItems" and "child_items" both select "ChildItems"; all
// selects every expandable path, wherever it stands among the names; an
// item left empty names nothing; and a name the model does not expand is
// refused, "invalid query parameter "_expand": unknown association
// "bogus"". _depth (default 1, as is a depth of zero) repeats the to-many
// associations holding the model itself for recursive preloading (see
// modelschema.RecursiveAssociations), e.g. expand "Children" with depth 3
// becomes "Children.Children.Children"; every other association ignores
// depth, its rows having no such association to descend into. A depth that
// is no count is refused the way urlquery.ParseCount words it, and one past
// urlquery.MaxExpandDepth with "invalid query parameter "_depth": expect a
// value from 1 to 10, got "11"", whether or not the request expands
// anything.
func parseExpandQuery(query url.Values, m types.Model) ([]string, error) {
	depth := 1
	if raw := query.Get(types.QueryDepth); raw != "" {
		asked, err := urlquery.ParseCount(types.QueryDepth, raw)
		switch {
		case err != nil:
			return nil, err
		case asked > urlquery.MaxExpandDepth:
			return nil, errors.Newf("invalid query parameter %q: expect a value from 1 to %d, got %q", types.QueryDepth, urlquery.MaxExpandDepth, raw)
		case asked > 0:
			depth = asked
		}
	}
	raw := query.Get(types.QueryExpand)
	if raw == "" {
		return nil, nil
	}

	expandable := m.Expands()
	selected := make([]bool, len(expandable))
	for item := range strings.SplitSeq(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		known := item == expandAll
		for i, e := range expandable {
			if item == expandAll || strings.EqualFold(strcase.SnakeCase(item), strcase.SnakeCase(e)) {
				selected[i], known = true, true
			}
		}
		if !known {
			return nil, errors.Newf("invalid query parameter %q: unknown association %q", types.QueryExpand, item)
		}
	}

	recursive := modelschema.RecursiveAssociations(reflect.TypeOf(m))
	var expands []string
	for at, e := range expandable {
		if !selected[at] {
			continue
		}
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
	return expands, nil
}
