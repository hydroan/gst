package pbschema

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGoCamelCaseNamesLikeTheProtobufPlugin pins the examples of the
// goCamelCase doc comment.
func TestGoCamelCaseNamesLikeTheProtobufPlugin(t *testing.T) {
	for name, want := range map[string]string{
		"sort_by":       "SortBy",
		"box_id":        "BoxId",
		"x_id":          "XId",
		"foo_2bar":      "Foo_2Bar",
		"_summary":      "XSummary",
		"Record":        "Record",
		"Record.Window": "Record_Window",
	} {
		require.Equal(t, want, goCamelCase(name), name)
	}
}
