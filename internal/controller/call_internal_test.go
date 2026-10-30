package controller

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
		"name":                 {"alice"},
		"age[gt]":              {"20"},
		"status[in]":           {"active,archived"},
		types.QuerySortBy:      {"name,created_at desc"},
		types.QueryPage:        {"2"},
		types.QuerySize:        {"50"},
		types.QueryCursorField: {"id"},
		types.QueryCursorValue: {"r-1"},
		types.QueryCursorNext:  {"true"},
		types.QueryExpand:      {"children,parent"},
		types.QueryDepth:       {"3"},
	}, values)

	empty, err := Query{}.values()
	require.NoError(t, err)
	require.Empty(t, empty)
}

// TestQueryValuesRefuseBeforeParsing pins the queries refused before any
// parsing: a filter given twice, refused first and the way the HTTP listener
// refuses a repeated parameter, every key so given listed; several values
// under an operator taking one; a member of an in holding a comma, which the
// HTTP parser would split; a filter without a value, or with an empty one,
// which over HTTP would filter by nothing; a filter without a field, or with
// one named like the framework's own parameters; and a sort or expand member
// holding a comma, which the HTTP parser would split too.
func TestQueryValuesRefuseBeforeParsing(t *testing.T) {
	for _, tt := range []struct {
		name    string
		query   Query
		message string
	}{
		{
			name:    "a filter given twice",
			query:   Query{Filters: []Filter{{Field: "name", Op: "eq", Values: []string{"a"}}, {Field: "name", Op: "eq", Values: []string{"b"}}}},
			message: `duplicate query parameter: name[eq]`,
		},
		{
			name: "two filters each given twice",
			query: Query{Filters: []Filter{
				{Field: "name", Values: []string{"a"}},
				{Field: "age", Op: "gt", Values: []string{"1"}},
				{Field: "name", Values: []string{"b"}},
				{Field: "age", Op: "gt", Values: []string{"2"}},
			}},
			message: `duplicate query parameter: age[gt], name`,
		},
		{
			name: "a filter given twice refused ahead of an empty value before it",
			query: Query{Filters: []Filter{
				{Field: "age", Op: "gt", Values: []string{""}},
				{Field: "name", Values: []string{"a"}},
				{Field: "name", Values: []string{"b"}},
			}},
			message: `duplicate query parameter: name`,
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

// TestFailServiceTellsACancellationFromAPassedDeadline pins what failService
// makes of a delegated service's error once the call ended: a call the
// client canceled answers Canceled with the error unlogged, a call whose
// deadline passed answers DeadlineExceeded with the error logged, the
// server's failure to answer in time; a live call answers the status the
// error maps to, logged as well.
func TestFailServiceTellsACancellationFromAPassedDeadline(t *testing.T) {
	cause := errors.New("dial tcp: connection refused")
	span := trace.SpanFromContext(context.Background())

	t.Run("a live call answers the mapped status and logs an error", func(t *testing.T) {
		rec := &levelRecorder{}
		c := &call{ctx: context.Background(), span: span, log: rec}
		require.Equal(t, codes.Internal, status.Code(c.failService(cause)))
		require.Equal(t, []string{"service operation failed"}, rec.errors)
	})

	t.Run("a canceled call answers Canceled unlogged", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		rec := &levelRecorder{}
		c := &call{ctx: ctx, span: span, log: rec}
		require.Equal(t, codes.Canceled, status.Code(c.failService(cause)))
		require.Empty(t, rec.errors, "a client that went away is not a failure of the server")
	})

	t.Run("a call whose deadline passed answers DeadlineExceeded and logs an error", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()
		rec := &levelRecorder{}
		c := &call{ctx: ctx, span: span, log: rec}
		require.Equal(t, codes.DeadlineExceeded, status.Code(c.failService(cause)))
		require.Equal(t, []string{"service operation failed"}, rec.errors, "the server did not answer in time: its failure, not the client's")
	})
}
