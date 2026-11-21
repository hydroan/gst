package urlquery_test

import (
	"math"
	"net/url"
	"strconv"
	"testing"

	"github.com/hydroan/gst/internal/urlquery"
	"github.com/stretchr/testify/require"
)

func TestWithoutExportKeys(t *testing.T) {
	q := url.Values{"name": {"alice"}, "_format": {"csv"}, "_limit": {"10"}}
	require.Equal(t, url.Values{"name": {"alice"}}, urlquery.WithoutExportKeys(q))
	require.Equal(t, url.Values{"name": {"alice"}, "_format": {"csv"}, "_limit": {"10"}}, q, "the query it is given stays as it is")
}

// TestParseCount guards the examples of ParseCount and pins what a count is:
// a whole number of zero or more that an int holds. A negative one is
// refused as one, and anything else it cannot read in the words of
// NotNumericError.
func TestParseCount(t *testing.T) {
	for raw, want := range map[string]int{"0": 0, "7": 7, strconv.Itoa(math.MaxInt): math.MaxInt} {
		count, err := urlquery.ParseCount("_page", raw)
		require.NoError(t, err, raw)
		require.Equal(t, want, count, raw)
	}

	_, err := urlquery.ParseCount("_page", "-1")
	require.EqualError(t, err, `invalid query parameter "_page": expect a non-negative value, got "-1"`)

	pastInt := strconv.FormatUint(uint64(math.MaxInt)+1, 10)
	for _, raw := range []string{"abc", "1.5", "", pastInt} {
		_, err := urlquery.ParseCount("_page", raw)
		require.EqualError(t, err, urlquery.NotNumericError("_page", raw).Error(), raw)
	}
}
