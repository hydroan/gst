package urlquery_test

import (
	"net/url"
	"testing"

	"github.com/hydroan/gst/internal/urlquery"
	"github.com/stretchr/testify/require"
)

func TestWithoutExportKeys(t *testing.T) {
	q := url.Values{"name": {"alice"}, "_format": {"csv"}, "_limit": {"10"}}
	require.Equal(t, url.Values{"name": {"alice"}}, urlquery.WithoutExportKeys(q))
	require.Equal(t, url.Values{"name": {"alice"}, "_format": {"csv"}, "_limit": {"10"}}, q, "the query it is given stays as it is")
}
