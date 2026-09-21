package urlquery

import (
	"net/url"
	"testing"
)

// BenchmarkFilters measures parsing the operator filters of one request — the
// per-request price of the "field[op]=value" syntax, paid by every list,
// export, and self-parsed service request that carries filters.
func BenchmarkFilters(b *testing.B) {
	query := url.Values{
		"name":            {"sample"},
		"age[gte]":        {"1"},
		"age[lt]":         {"100"},
		"remark[like]":    {"wor"},
		"remark[isnull]":  {"false"},
		"item_count[in]":  {"1,2,3"},
		"enabled[eq]":     {"true"},
		"expired_at[gte]": {"2020-01-01T00:00:00Z"},
	}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := Filters(query, &filterTestModel{}); err != nil {
			b.Fatal(err)
		}
	}
}
