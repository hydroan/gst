package urlquery

import (
	"net/url"
	"testing"

	"github.com/gorilla/schema"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

// The gorilla/schema pipeline below is the reference implementation Decode
// replaced, kept test-only so the comparison benchmark stays honest and the
// library never links into production binaries again.

var gorillaDecoder = func() *schema.Decoder {
	decoder := schema.NewDecoder()
	decoder.SetAliasTag("query")
	return decoder
}()

// decodeGorilla replays the replaced pipeline: copy the query without filter
// keys, drop the controller-owned parameters, and stream-decode the rest.
func decodeGorilla(q url.Values, m types.Model) error {
	values := make(url.Values, len(q))
	for key, entry := range q {
		if isFilterQueryKey(key) {
			continue
		}
		values[key] = entry
	}
	delete(values, consts.QUERY_FORMAT)
	if !modelregistry.IsPaginatable(m) {
		delete(values, consts.QUERY_SIZE)
	}
	return gorillaDecoder.Decode(m, values)
}

// benchmarkDecodeValues is one representative request: model conditions of
// every scalar shape both pipelines resolve, framework paging and sorting
// parameters, and filter keys the decoder must step around.
var benchmarkDecodeValues = url.Values{
	"name":            {"sample"},
	"age":             {"10"},
	"remark":          {"hello"},
	"enabled":         {"true"},
	"_page":           {"2"},
	"_size":           {"50"},
	"_sort_by":        {"created_at desc"},
	"age[gte]":        {"1"},
	"remark[like]":    {"hel"},
	"created_at[gte]": {"2020-01-01T00:00:00Z"},
}

// BenchmarkDecode measures the per-request cost of the plan-based decoder on
// the representative request above.
func BenchmarkDecode(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		var m filterTestModel
		if err := Decode(benchmarkDecodeValues, &m); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDecodeGorilla measures the gorilla/schema pipeline this package
// replaced, on the same request, for a like-for-like comparison.
func BenchmarkDecodeGorilla(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		var m filterTestModel
		if err := decodeGorilla(benchmarkDecodeValues, &m); err != nil {
			b.Fatal(err)
		}
	}
}
