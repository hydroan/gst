package urlquery_test

import (
	"net/url"
	"testing"

	"github.com/hydroan/gst/internal/urlquery"
)

// BenchmarkPagination measures reading the page and size of a request that
// names neither, the common list request, which takes the defaults.
func BenchmarkPagination(b *testing.B) {
	query := url.Values{"name": {"sample"}}

	b.ReportAllocs()
	for b.Loop() {
		urlquery.Pagination(query)
	}
}
