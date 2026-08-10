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
	"net/url"
	"strings"

	"github.com/stoewer/go-strcase"
)

// PresentFields collects the model filter keys explicitly provided in the URL
// query string, keyed by snake case column name, so the database layer can
// keep zero values (false, 0) of these columns as query conditions. Framework
// parameters (the "_" prefix namespace) and keys whose values are all empty
// are excluded: they are not model filter columns, and an empty value means
// the caller is not filtering by that key.
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
		present[strcase.SnakeCase(key)] = struct{}{}
	}
	return present
}
