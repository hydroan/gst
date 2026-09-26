package controller

import (
	"net/url"
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/stretchr/testify/require"
)

// TestQueryValuesRenderTheHTTPQuery pins how the query of a call becomes
// the query string the HTTP listener parses: a filter with an operator as
// field[op]=value, one without as the bare key, the members of an in joined
// by commas, the orderings joined under _sort_by, and the paging, cursor and
// expansion under their parameters, each only when set.
func TestQueryValuesRenderTheHTTPQuery(t *testing.T) {
	values, err := Query{
		Filters: []Filter{
			{Field: "name", Values: []string{"alice"}},
			{Field: "age", Op: "gt", Values: []string{"20"}},
			{Field: "status", Op: "in", Values: []string{"active", "archived"}},
		},
		SortBy:      []string{"name", "created_at desc"},
		Page:        2,
		Size:        50,
		CursorField: "id",
		CursorValue: "r-1",
		CursorNext:  true,
		Expand:      []string{"children", "parent"},
		Depth:       3,
	}.values()

	require.NoError(t, err)
	require.Equal(t, url.Values{
		"name":                    {"alice"},
		"age[gt]":                 {"20"},
		"status[in]":              {"active,archived"},
		consts.QUERY_SORT_BY:      {"name,created_at desc"},
		consts.QUERY_PAGE:         {"2"},
		consts.QUERY_SIZE:         {"50"},
		consts.QUERY_CURSOR_FIELD: {"id"},
		consts.QUERY_CURSOR_VALUE: {"r-1"},
		consts.QUERY_CURSOR_NEXT:  {"true"},
		consts.QUERY_EXPAND:       {"children,parent"},
		consts.QUERY_DEPTH:        {"3"},
	}, values)

	empty, err := Query{}.values()
	require.NoError(t, err)
	require.Empty(t, empty)
}

// TestQueryValuesRefuseWhatTheHTTPQueryCannotCarry pins the queries refused
// before any parsing: a filter given twice, which the HTTP listener refuses
// as a repeated parameter; several values under an operator taking one; a
// member of an in holding a comma, which the HTTP parser would split; and a
// filter without a value, which filters by nothing.
func TestQueryValuesRefuseWhatTheHTTPQueryCannotCarry(t *testing.T) {
	for _, tt := range []struct {
		name    string
		filters []Filter
		message string
	}{
		{
			name:    "a filter given twice",
			filters: []Filter{{Field: "name", Op: "eq", Values: []string{"a"}}, {Field: "name", Op: "eq", Values: []string{"b"}}},
			message: `filter "name[eq]" is given twice`,
		},
		{
			name:    "several values under an operator taking one",
			filters: []Filter{{Field: "name", Values: []string{"a", "b"}}},
			message: `filter "name" takes one value, 2 given; in and notin take several`,
		},
		{
			name:    "a member of an in holding a comma",
			filters: []Filter{{Field: "name", Op: "notin", Values: []string{"a,b"}}},
			message: `filter "name[notin]": a value cannot hold a comma, the members are joined by it`,
		},
		{
			name:    "a filter without a value",
			filters: []Filter{{Field: "remark", Op: "like"}},
			message: `filter "remark[like]" has no value`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Query{Filters: tt.filters}.values()
			require.EqualError(t, err, tt.message)
		})
	}
}
