package controller

import (
	"net/url"
	"reflect"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/model"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/internal/urlquery"
	"github.com/stoewer/go-strcase"
)

// expandAll is the value of the expand parameter asking for every
// association.
const expandAll = "all"

// listQueryKeys are the List parameters model.Query opts a model in to,
// beside the operator filters.
var listQueryKeys = map[string]struct{}{
	types.QueryExpand: {},
	types.QueryDepth:  {},
	types.QuerySortBy: {},
}

// listPagingQueryKeys are the List parameters a list pages by: the size of
// every list, and the page of one that is no feed. An export, which lists
// every record the query matches, refuses them beside the cursor's.
var listPagingQueryKeys = map[string]struct{}{
	types.QueryPage: {},
	types.QuerySize: {},
}

// listCursorQueryKeys are the List parameters a feed alone takes, the list of
// a model embedding model.Cursor.
var listCursorQueryKeys = map[string]struct{}{
	types.QueryCursorValue: {},
	types.QueryCursorField: {},
	types.QueryCursorNext:  {},
}

// decodeListQuery rejects the framework query keys the model has not opted
// in to, the keys of model.Query and those of model.Cursor, then decodes the
// remaining URL query parameters into the model's own query fields. The
// explicit gate reports every such key in one error. _page and _size are
// every list's but a feed's page, which urlquery.Cursor refuses beside
// _sort_by when the flow reads the cursor.
func decodeListQuery[M types.Model](m M, query map[string][]string) error {
	rejected := make([]string, 0)
	if !model.IsQueryable(m) {
		rejected = append(rejected, matchQueryKeys(query, listQueryKeys)...)
	}
	if !model.IsCursorable(m) {
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
		asked, err := urlquery.ParseCount(types.QueryDepth, raw, urlquery.MaxExpandDepth)
		if err != nil {
			return nil, err
		}
		if asked > 0 {
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
