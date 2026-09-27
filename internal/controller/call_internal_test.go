package controller

import (
	"net/url"
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/stretchr/testify/require"
)

// TestQueryValuesRenderTheHTTPQuery pins the example of the Query.values
// doc comment, the same query in and the same query string out: a filter
// with an operator as field[op]=value, one without as the bare key, the
// members of an in joined by commas, the orderings joined under _sort_by,
// and the paging, cursor and expansion under their parameters, each only
// when set.
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
// member of an in holding a comma, which the HTTP parser would split; a
// filter without a value, or with an empty one, which filters by nothing;
// a field named like the framework's own parameters; and a sort or expand
// member holding a comma, which the HTTP parser would split too.
func TestQueryValuesRefuseWhatTheHTTPQueryCannotCarry(t *testing.T) {
	for _, tt := range []struct {
		name    string
		query   Query
		message string
	}{
		{
			name:    "a filter given twice",
			query:   Query{Filters: []Filter{{Field: "name", Op: "eq", Values: []string{"a"}}, {Field: "name", Op: "eq", Values: []string{"b"}}}},
			message: `filter "name[eq]" is given twice`,
		},
		{
			name:    "several values under an operator taking one",
			query:   Query{Filters: []Filter{{Field: "name", Values: []string{"a", "b"}}}},
			message: `filter "name" takes one value, 2 given; in and notin take several`,
		},
		{
			name:    "a member of an in holding a comma",
			query:   Query{Filters: []Filter{{Field: "name", Op: "notin", Values: []string{"a,b"}}}},
			message: `filter "name[notin]": a value cannot hold a comma, the members are joined by it`,
		},
		{
			name:    "a filter without a value",
			query:   Query{Filters: []Filter{{Field: "remark", Op: "like"}}},
			message: `filter "remark[like]" has no value`,
		},
		{
			name:    "a filter with an empty value",
			query:   Query{Filters: []Filter{{Field: "name", Values: []string{""}}}},
			message: `filter "name" has an empty value`,
		},
		{
			name:    "a filter named like a framework parameter",
			query:   Query{Filters: []Filter{{Field: "_page", Values: []string{"2"}}}},
			message: `filter "_page": a field cannot start with an underscore, the framework's own parameters do`,
		},
		{
			name:    "a filter without a field",
			query:   Query{Filters: []Filter{{Values: []string{"a"}}}},
			message: `filter "": a field is required`,
		},
		{
			name:    "a sort member holding a comma",
			query:   Query{SortBy: []string{"name,created_at"}},
			message: `sort_by: a member cannot hold a comma, the members are joined by it`,
		},
		{
			name:    "an expand member holding a comma",
			query:   Query{Expand: []string{"children,parent"}},
			message: `expand: a member cannot hold a comma, the members are joined by it`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.query.values()
			require.EqualError(t, err, tt.message)
		})
	}
}
