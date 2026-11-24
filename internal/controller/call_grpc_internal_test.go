package controller

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/gst"
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
		"name":               {"alice"},
		"age[gt]":            {"20"},
		"status[in]":         {"active,archived"},
		gst.QuerySortBy:      {"name,created_at desc"},
		gst.QueryPage:        {"2"},
		gst.QuerySize:        {"50"},
		gst.QueryCursorField: {"id"},
		gst.QueryCursorValue: {"r-1"},
		gst.QueryCursorNext:  {"true"},
		gst.QueryExpand:      {"children,parent"},
		gst.QueryDepth:       {"3"},
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
// one named like the framework's own parameters; a sort or expand member
// holding a comma, which the HTTP parser would split too; and a size past
// the cap, in the words the HTTP listener refuses it at its door.
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
			name:    "a size past the cap",
			query:   Query{Size: 500},
			message: `invalid query parameter "_size": expect a value from 1 to 100, got "500"`,
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

// TestACallRefusedForItsMessageIsReportedAsTheClients pins the level the
// refusals of a call's own making are logged at: a message naming no record
// and a stream message the decoding refused are the client's doing, logged
// at info under the message of a rejected request; the decoding's refusal
// stays the status the stream answers, as it was received.
func TestACallRefusedForItsMessageIsReportedAsTheClients(t *testing.T) {
	span := trace.SpanFromContext(context.Background())

	t.Run("a message naming no record", func(t *testing.T) {
		rec := &levelRecorder{}
		c := &call{ctx: context.Background(), span: span, log: rec}
		require.Equal(t, codes.InvalidArgument, status.Code(c.missingID()))
		require.Equal(t, []string{"request message rejected"}, rec.infos)
		require.Empty(t, rec.errors, "a refusal the client brought on itself is not a failure of the server")
	})

	t.Run("a stream message the decoding refused", func(t *testing.T) {
		rec := &levelRecorder{}
		c := &call{ctx: context.Background(), span: span, log: rec}
		a := newAction[*flowSample, *flowSample, *flowSample]("flow-samples", apiaction.Stream)
		refusal := status.Error(codes.InvalidArgument, "invalid value for field 'rank'")
		in := a.requests(c, func() (*flowSample, error) { return nil, refusal })

		_, err := in.recv()

		require.Same(t, refusal, err)
		require.Same(t, refusal, in.refusal())
		require.Equal(t, []string{"request message rejected"}, rec.infos)
		require.Empty(t, rec.errors, "a refusal the client brought on itself is not a failure of the server")
	})
}

// TestFailServiceTellsACancellationFromAPassedDeadline pins what failService
// makes of a delegated service's error once the call ended: a call the
// client canceled answers Canceled with the error unlogged, a call whose
// deadline passed answers DeadlineExceeded with the error logged, the
// server's failure to answer in time; a live call answers the status the
// error maps to, logged at the level that answer calls for, the 408 of a
// client slow to send at warn though it answers the code of a deadline that
// passed. A live call whose stream reported the client gone is a canceled
// one too, whatever its service returns: grpc ends a stream before it
// cancels the stream's context, so a Send can fail Canceled while the
// context still says nothing (see call.streamErr). A Canceled the service
// returns with no such report is a failure of its own, a call it canceled
// downstream, answered and logged like any other.
func TestFailServiceTellsACancellationFromAPassedDeadline(t *testing.T) {
	cause := errors.New("dial tcp: connection refused")
	span := trace.SpanFromContext(context.Background())

	t.Run("a live call answers the mapped status and logs an error", func(t *testing.T) {
		rec := &levelRecorder{}
		c := &call{ctx: context.Background(), span: span, log: rec}
		require.Equal(t, codes.Internal, status.Code(c.failService(cause)))
		require.Equal(t, []string{"service operation failed"}, rec.errors)
	})

	t.Run("a live call the service refused answers the refusal and logs below error", func(t *testing.T) {
		rec := &levelRecorder{}
		c := &call{ctx: context.Background(), span: span, log: rec}
		require.Equal(t, codes.NotFound, status.Code(c.failService(gst.NewError(http.StatusNotFound, "no such record"))))
		require.Equal(t, []string{"service operation failed"}, rec.infos)
		require.Empty(t, rec.errors, "a refusal the client brought on itself is not a failure of the server")
	})

	t.Run("a live call answering 408 logs a warning, not the error of a passed deadline", func(t *testing.T) {
		rec := &levelRecorder{}
		c := &call{ctx: context.Background(), span: span, log: rec}
		require.Equal(t, codes.DeadlineExceeded, status.Code(c.failService(gst.NewError(http.StatusRequestTimeout, requestTimeoutMsg))))
		require.Equal(t, []string{"service operation failed"}, rec.warns)
		require.Empty(t, rec.errors, "the client was slow to send: its doing, not the server's")
	})

	t.Run("a canceled call answers Canceled unlogged", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		rec := &levelRecorder{}
		c := &call{ctx: ctx, span: span, log: rec}
		require.Equal(t, codes.Canceled, status.Code(c.failService(cause)))
		require.Empty(t, rec.errors, "a client that went away is not a failure of the server")
	})

	t.Run("a live call whose stream reported the client gone answers Canceled unlogged, whatever the service returns", func(t *testing.T) {
		gone := status.Error(codes.Canceled, context.Canceled.Error())
		for name, returned := range map[string]error{
			"the stream's error as it came": gone,
			"the stream's error wrapped":    errors.Wrap(gone, "send the next record"),
			"an error of the service's own": cause,
		} {
			rec := &levelRecorder{}
			c := &call{ctx: context.Background(), span: span, log: rec}
			require.Equal(t, gone, c.streamErr(gone), name)
			require.Equal(t, codes.Canceled, status.Code(c.failService(returned)), name)
			require.Empty(t, rec.errors, "a client that went away is not a failure of the server: %s", name)
			require.Empty(t, rec.warns, name)
			require.Empty(t, rec.infos, name)
		}
	})

	t.Run("a live call returning a Canceled no stream reported answers the mapped status and logs an error", func(t *testing.T) {
		rec := &levelRecorder{}
		c := &call{ctx: context.Background(), span: span, log: rec}
		require.Equal(t, codes.Internal, status.Code(c.failService(status.Error(codes.Canceled, context.Canceled.Error()))))
		require.Equal(t, []string{"service operation failed"}, rec.errors, "a call the service canceled downstream is the server's failure, as the HTTP line answers it")
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
