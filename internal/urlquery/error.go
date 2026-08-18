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
