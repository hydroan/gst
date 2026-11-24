// Package urlquery translates URL query parameters into the arguments the
// database layer builds a query from.
//
// The package owns the parsing of one input, url.Values, and produces nothing
// but database query primitives: filter conditions, present field markers,
// pagination and cursor arguments. It knows nothing about HTTP handlers or
// services, so both the framework list controllers and application services
// parse a request exactly the same way.
//
// Which parameters a request may use is decided by the model: every list
// pages by number, a model embedding model.Cursor has its list read by
// cursor instead, and model.Query opts in to sorting, expansion and the
// operator filters. The parameter readers (Pagination, Cursor, Orders) read
// what the model takes and leave the rest alone; refusing the parameters of
// a capability the model lacks is the list controller's job, not this
// package's, but for the page and the order of a feed, which Cursor refuses
// wherever a feed is read. Keys that map to nothing are different and always
// fail — in Decode and Filters alike — because silently dropping a mistyped
// filter would widen the result set.
package urlquery

import (
	"maps"
	"net/url"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/modelschema"
)

// WithoutExportKeys returns a copy of q without the export action's own
// parameters, _format and _limit, which the export reads itself before it
// hands the copy to the parsers; they refuse both keys from any other
// action. For name=a&_format=csv&_limit=10 it returns name=a, q left as it is.
func WithoutExportKeys(q url.Values) url.Values {
	kept := maps.Clone(q)
	delete(kept, gst.QueryFormat)
	delete(kept, gst.QueryLimit)
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

// MaxExpandDepth is the largest _depth a request may ask for. Every level of
// depth becomes one more recursive preload query, so the cap keeps a single
// request from fanning out unbounded database work. The controllers refuse a
// deeper one, and the pb generator quotes the cap in the comment of a
// request's depth field, so a gRPC caller reads it off the message.
const MaxExpandDepth = 10

// ParseCount reads the count a query parameter carries, the value of _page,
// _size or _depth: a whole number of zero or more, up to most, the largest
// the parameter takes, math.MaxInt for one without a cap, an int being the
// type the readers of a count hold it in. It reads the way
// modelschema.ParseUint reads an unsigned integer and refuses in its words
// under the parameter's name: given "_page" and "-1" it reports
//
//	invalid query parameter "_page": expect a non-negative value, got "-1"
//
// given a value that is no number, what NotNumericError reports, and given
// "_size", "500" and a most of 100, zero being the parameter left out,
//
//	invalid query parameter "_size": expect a value from 1 to 100, got "500"
func ParseCount(key, raw string, most int) (int, error) {
	parsed, err := modelschema.ParseUint(raw, strconv.IntSize-1)
	if err != nil {
		return 0, errors.New(invalidParameterMessage(key, err))
	}
	count := int(parsed) //nolint:gosec // G115: read into one bit less than an int has, the count is within its range.
	if count > most {
		return 0, errors.Newf("invalid query parameter %q: expect a value from 1 to %d, got %q", key, most, raw)
	}
	return count, nil
}
