package urlquery_test

import (
	"testing"

	"github.com/hydroan/gst/internal/urlquery"
	"github.com/stretchr/testify/require"
)

// TestDuplicateParameterError guards the example of DuplicateParameterError:
// the keys given twice are listed sorted, and the keys handed in keep their
// order.
func TestDuplicateParameterError(t *testing.T) {
	keys := []string{"name[eq]", "age[gt]"}

	require.EqualError(t, urlquery.DuplicateParameterError(keys), "duplicate query parameter: age[gt], name[eq]")
	require.Equal(t, []string{"name[eq]", "age[gt]"}, keys, "the keys handed in keep their order")
}
