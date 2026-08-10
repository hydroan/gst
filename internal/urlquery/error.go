package urlquery

import (
	"fmt"
	"net/url"
	"reflect"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/gorilla/schema"
)

// UnsupportedParameterError reports query keys the request may not use, either
// because they map to no query field of the model or because the model did not
// opt in to the capability owning them. Every offending key is listed, sorted,
// so the client sees the full set at once.
func UnsupportedParameterError(keys []string) error {
	return errors.New(unsupportedParameterMessage(keys))
}

// unsupportedParameterMessage renders the sorted key list shared by
// UnsupportedParameterError and translateDecodeError.
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

// translateDecodeError rewrites gorilla/schema's decode errors into the
// package's own wording. The decoder renders one error plus a count ("(and 2
// other errors)") and speaks in library terms ("schema: invalid path"),
// neither of which tells a client what to fix; the translation lists every
// offending key and names the expected value type. Errors other than the
// decoder's MultiError pass through unchanged.
func translateDecodeError(values url.Values, err error) error {
	var multi schema.MultiError
	if !errors.As(err, &multi) {
		return err
	}
	unsupported := make([]string, 0, len(multi))
	invalid := make([]string, 0, len(multi))
	for key, entry := range multi {
		var unknownKey schema.UnknownKeyError
		if errors.As(entry, &unknownKey) {
			unsupported = append(unsupported, key)
			continue
		}
		invalid = append(invalid, invalidParameterMessage(values, key, entry))
	}
	// The uniform "invalid query parameter" prefix makes sorting the rendered
	// messages equivalent to sorting by key.
	sort.Strings(invalid)
	parts := make([]string, 0, len(invalid)+1)
	if len(unsupported) > 0 {
		parts = append(parts, unsupportedParameterMessage(unsupported))
	}
	parts = append(parts, invalid...)
	return errors.New(strings.Join(parts, "; "))
}

// invalidParameterMessage renders one MultiError entry for a key that maps to
// a model field but whose value does not parse. The expected type is phrased
// like the filter and cursor validators ("expect a numeric value"), and the
// offending value comes from the query itself: gorilla/schema's basic
// converters fail without an underlying error, so the entry alone cannot name
// it.
func invalidParameterMessage(values url.Values, key string, entry error) string {
	var conv schema.ConversionError
	if errors.As(entry, &conv) && conv.Type != nil {
		got := rawQueryValue(values, key, conv.Index)
		switch {
		case isNumericKind(conv.Type.Kind()):
			return fmt.Sprintf("invalid query parameter %q: expect a numeric value, got %q", key, got)
		case conv.Type.Kind() == reflect.Bool:
			return fmt.Sprintf("invalid query parameter %q: expect a boolean value, got %q", key, got)
		default:
			return fmt.Sprintf("invalid query parameter %q: cannot parse %q as %s", key, got, conv.Type)
		}
	}
	return fmt.Sprintf("invalid query parameter %q: %s", key, strings.TrimPrefix(entry.Error(), "schema: "))
}

// rawQueryValue returns the query value a MultiError entry refers to; the
// index points into multi-value keys and is negative for single-value ones.
func rawQueryValue(values url.Values, key string, index int) string {
	all := values[key]
	if index < 0 {
		index = 0
	}
	if index >= len(all) {
		return ""
	}
	return all[index]
}
