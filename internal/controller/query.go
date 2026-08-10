package controller

import (
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/urlquery"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
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

// maxExpandDepth caps the _depth parameter. Every depth level becomes one
// more recursive preload query, so the cap keeps a single request from
// fanning out unbounded database work.
const maxExpandDepth = 10

// modelFieldKindsCache caches the field-name-to-kind mapping per model type.
// The mapping is pure type information, so it is computed once per type
// instead of on every expanding request; cached maps are read-only.
var modelFieldKindsCache sync.Map // reflect.Type -> map[string]reflect.Kind

// cachedModelFieldKinds returns the struct field kinds of the type keyed by
// field name, computing and caching them on first use. parseExpandQuery uses
// the kinds to repeat slice associations for recursive preloading.
func cachedModelFieldKinds(typ reflect.Type) map[string]reflect.Kind {
	if cached, ok := modelFieldKindsCache.Load(typ); ok {
		return cached.(map[string]reflect.Kind) //nolint:errcheck
	}
	fieldKinds := make(map[string]reflect.Kind, typ.NumField())
	for field := range typ.Fields() {
		fieldKinds[field.Name] = field.Type.Kind()
	}
	modelFieldKindsCache.Store(typ, fieldKinds)
	return fieldKinds
}

// parseExpandQuery resolves the _expand and _depth query parameters against
// the model's expandable association paths. Expand names are matched against
// m.Expands() ignoring case and snake case punctuation, so "childItems" and
// "child_items" both select "ChildItems"; "_expand=all" selects every
// expandable path. _depth (clamped to [1,10], default 1) repeats slice
// associations for recursive preloading, e.g. expand "Children" with depth 3
// becomes "Children.Children.Children"; non-slice associations ignore depth.
func parseExpandQuery(c *gin.Context, m types.Model) []string {
	expandStr, ok := c.GetQuery(consts.QUERY_EXPAND)
	if !ok {
		return nil
	}
	depth := 1
	if depthStr, ok := c.GetQuery(consts.QUERY_DEPTH); ok {
		depth, _ = strconv.Atoi(depthStr)
		if depth < 1 || depth > maxExpandDepth {
			depth = 1
		}
	}

	items := strings.Split(expandStr, ",")
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

	fieldsMap := cachedModelFieldKinds(reflect.TypeOf(m).Elem())
	var expands []string
	for _, e := range matched {
		// If the expanding field does not exist in the structure fields, skip depth expand.
		kind, found := fieldsMap[e]
		if !found {
			expands = append(expands, e)
			continue
		}
		// If the expanding field exists in the structure but the kind is not slice, skip depth expand.
		if kind != reflect.Slice {
			expands = append(expands, e)
			continue
		}
		t := make([]string, depth)
		for i := range depth {
			t[i] = e
		}
		// If expand="Children" and depth=3, the depth expanded is "Children.Children.Children".
		expands = append(expands, strings.Join(t, "."))
	}
	return expands
}
