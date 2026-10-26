// Package urlquery translates URL query parameters into the arguments the
// database layer builds a query from.
//
// The package owns the parsing of one input, url.Values, and produces nothing
// but database query primitives: filter conditions, present field markers,
// pagination and cursor arguments. It knows nothing about HTTP handlers or
// services, so both the framework list controllers and application services
// parse a request exactly the same way.
//
// Which parameters a request may use is decided by the model: capabilities are
// opted in to by embedding model.Query, model.Pagination or model.Cursor. The
// parameter readers (Pagination, Cursor, Orders) ignore parameters the model
// did not opt in to and fall back to the framework defaults rather than
// failing the request; rejecting those capability keys is the list
// controller's job, not this package's. Keys that map to nothing are different
// and always fail — in Decode and Filters alike — because silently dropping a
// mistyped filter would widen the result set.
package urlquery

import (
	"maps"
	"net/url"
	"strings"

	"github.com/hydroan/gst/internal/consts"
)

// WithoutExportKeys returns a copy of q without the export action's own
// parameters, _format and _limit, which the export reads itself before it
// hands the copy to the parsers; they refuse both keys from any other
// action. For name=a&_format=csv&_limit=10 it returns name=a, q left as it is.
func WithoutExportKeys(q url.Values) url.Values {
	kept := maps.Clone(q)
	delete(kept, consts.QUERY_FORMAT)
	delete(kept, consts.QUERY_LIMIT)
	return kept
}

// PresentFields collects the model filter keys explicitly provided in the URL
// query string, keyed as the query string spells them, the URL names the
// database layer looks a field up by, so it can keep zero values (false, 0)
// of these columns as query conditions. Framework parameters (the "_" prefix
// namespace) and keys whose values are all empty are excluded: they are not
// model filter columns, and an empty value means the caller is not filtering
// by that key.
func PresentFields(q url.Values) map[string]struct{} {
	present := make(map[string]struct{}, len(q))
	for key, values := range q {
		if strings.HasPrefix(key, "_") {
			continue
		}
		if isFilterKey(key) {
			continue
		}
		if len(strings.Join(values, "")) == 0 {
			continue
		}
		present[key] = struct{}{}
	}
	return present
}
