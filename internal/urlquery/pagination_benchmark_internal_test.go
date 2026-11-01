package urlquery

import (
	"net/url"
	"testing"
)

// BenchmarkPagination measures reading the page and size of a request that
// names neither, the common list request, which takes the defaults.
func BenchmarkPagination(b *testing.B) {
	query := url.Values{"name": {"sample"}}

	b.ReportAllocs()
	for b.Loop() {
		Pagination(query, &paginatableTestModel{})
	}
}
