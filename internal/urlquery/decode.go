package urlquery

import (
	"net/url"

	"github.com/gorilla/schema"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

// queryDecoder is the shared gorilla/schema decoder used to parse URL query
// parameters into models. A single instance is kept so gorilla/schema can
// reuse its internal struct-metadata cache across requests instead of
// rebuilding it via reflection every time.
//
// Decode is safe for concurrent use, but the option setters and converter
// registration are not; therefore the decoder is configured once here and is
// never reconfigured afterwards.
var queryDecoder = func() *schema.Decoder {
	decoder := schema.NewDecoder()
	decoder.SetAliasTag("query")

	return decoder
}()

// Decode fills the model's own query fields from the URL query, producing the
// query value WithQuery takes as its first argument.
//
// The keys Filters owns are dropped first, so the "field[op]" bracket syntax
// never reaches the schema decoder. _size is dropped as well when the model
// cannot carry it: a model embedding only Cursor accepts the parameter as its
// batch size, but the Size field lives in Pagination.
//
// Every remaining key must map to a field of the model, so a mistyped filter
// name is reported instead of silently widening the result set;
// translateDecodeError turns the decoder's raw errors into that report.
func Decode(q url.Values, m types.Model) error {
	values := withoutFilters(q)
	if !modelregistry.IsPaginatable(m) {
		delete(values, consts.QUERY_SIZE)
	}
	return translateDecodeError(values, queryDecoder.Decode(m, values))
}

// withoutFilters returns a copy of the query without the keys owned by
// Filters, so schema decoding of the model's own query fields never sees them.
func withoutFilters(q url.Values) url.Values {
	filtered := make(url.Values, len(q))
	for key, values := range q {
		if isFilterQueryKey(key) {
			continue
		}
		filtered[key] = values
	}
	return filtered
}
