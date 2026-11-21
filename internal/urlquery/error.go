package urlquery

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
)

// UnsupportedParameterError reports query keys the request may not use, either
// because they map to no query field of the model or because the model did not
// opt in to the capability owning them. Every offending key is listed, sorted,
// so the client sees the full set at once.
func UnsupportedParameterError(keys []string) error {
	return errors.New(unsupportedParameterMessage(keys))
}

// unsupportedParameterMessage renders the sorted key list shared by
// UnsupportedParameterError and Decode.
func unsupportedParameterMessage(keys []string) string {
	sorted := make([]string, len(keys))
	copy(sorted, keys)
	sort.Strings(sorted)
	quoted := make([]string, len(sorted))
	for i, key := range sorted {
		quoted[i] = fmt.Sprintf("%q", key)
	}
	noun := "parameter"
	if len(quoted) > 1 {
		noun = "parameters"
	}
	return fmt.Sprintf("unsupported query %s %s", noun, strings.Join(quoted, ", "))
}

// DuplicateParameterError reports query keys a request gives more than once:
// the framework reads one value per key and refuses a key given twice rather
// than pick one of its values. Both transports refuse with it, the HTTP
// listener a repeated parameter and a gRPC call a filter given twice. Every
// such key is listed, sorted, so the client sees the full set at once; given
// []string{"name[eq]", "age[gt]"} it reports
//
//	duplicate query parameter: age[gt], name[eq]
func DuplicateParameterError(keys []string) error {
	sorted := make([]string, len(keys))
	copy(sorted, keys)
	sort.Strings(sorted)
	return errors.New("duplicate query parameter: " + strings.Join(sorted, ", "))
}

// NotNumericError reports a query parameter that takes a number and is given
// a value spelling none, in the words Decode refuses such a value of a
// numeric model field with, for the readers of a parameter no model field
// holds. Given "_limit" and "abc" it reports
//
//	invalid query parameter "_limit": expect a numeric value, got "abc"
func NotNumericError(key, raw string) error {
	return errors.New(notNumericMessage(key, raw))
}

// notNumericMessage renders the message shared by NotNumericError and
// Decode.
func notNumericMessage(key, raw string) string {
	return fmt.Sprintf("invalid query parameter %q: expect a numeric value, got %q", key, raw)
}

// invalidParameterMessage renders the refusal of the value a query parameter
// is given, the parameter by name and then the reason a reader of the value
// refused it with, shared by Decode and ParseCount: for "enabled" and
// modelschema.ParseBool's refusal of "maybe",
//
//	invalid query parameter "enabled": expect a boolean value, got "maybe"
func invalidParameterMessage(key string, reason error) string {
	return fmt.Sprintf("invalid query parameter %q: %v", key, reason)
}
