package controller_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/controller"
	"github.com/stretchr/testify/require"
)

// TestListRefusesMalformedQueryParameters pins the 400 of a listing whose
// query parameters the model cannot answer, each refused before any row is
// read.
func TestListRefusesMalformedQueryParameters(t *testing.T) {
	samples := controller.ListHandler[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](sampleRoute))
	counters := controller.ListHandler[*sampleCounter, *sampleCounter, *sampleCounter](configFor[*sampleCounter](counterRoute))

	for _, tt := range []struct {
		name   string
		counts bool
		target string
	}{
		{name: "a page size on a model that does not page", counts: true, target: "/controller-counters?_size=10"},
		{name: "a filter on a field the model lacks", target: "/controller-samples?missing[eq]=value"},
		{name: "a filter operator that does not exist", target: "/controller-samples?name[bogus]=value"},
		{name: "a sort column the model lacks", target: "/controller-samples?_sort_by=missing"},
		{name: "a sort direction that does not exist", target: "/controller-samples?_sort_by=name+sideways"},
		{name: "a cursor column the model lacks", target: "/controller-samples?_cursor_field=missing&_cursor_value=value"},
		{name: "a cursor column the model lacks on the first page", target: "/controller-samples?_cursor_field=missing"},
		{name: "a cursor beside an order of its own", target: "/controller-samples?_cursor_field=id&_cursor_value=value&_sort_by=name"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			handler, pattern := samples, "/controller-samples"
			if tt.counts {
				handler, pattern = counters, "/controller-counters"
			}
			rsp := serve(t, http.MethodGet, pattern, handler, tt.target, "")

			require.Equal(t, http.StatusBadRequest, rsp.Code, rsp.Body.String())
		})
	}
}

// TestListAnswersAHookRefusal pins that a listing a list hook refuses answers
// with the hook's error instead of rows: ListBefore, and Filter, the hook a
// service scopes a listing to what its caller may see in.
func TestListAnswersAHookRefusal(t *testing.T) {
	name := uniqueName("list-refused")
	createSample(t, name)

	for _, route := range []string{refusalRoute, filterRefusalRoute} {
		t.Run(route, func(t *testing.T) {
			rsp := serve(t, http.MethodGet, "/"+route,
				controller.ListHandler[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](route)),
				"/"+route, "")

			require.Equal(t, http.StatusConflict, rsp.Code)
			require.Contains(t, rsp.Body.String(), refusedMsg)
			require.NotContains(t, rsp.Body.String(), name)
		})
	}
}

// TestListWalksAFeedToItsEnd pins what has_more spares the client reading a
// feed: four records read two a page take two requests, the second page
// telling that none follow it, and no third request for an empty page.
func TestListWalksAFeedToItsEnd(t *testing.T) {
	name := uniqueName("list-feed")
	created := make([]string, 0, 4)
	for range 4 {
		created = append(created, createSample(t, name).GetID())
	}
	handler := controller.ListHandler[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](sampleRoute))

	first := serveList(t, handler, "/"+sampleRoute, "/"+sampleRoute+"?name="+name+"&_size=2&_cursor_next=true")
	require.Len(t, first.Items, 2)
	require.NotNil(t, first.HasMore, "a list response carries has_more")
	require.True(t, *first.HasMore, "two records follow the first page")

	last := stringOf(first.Items[1]["id"])
	second := serveList(t, handler, "/"+sampleRoute, "/"+sampleRoute+"?name="+name+"&_size=2&_cursor_next=true&_cursor_value="+last)
	require.Len(t, second.Items, 2)
	require.NotNil(t, second.HasMore, "a list response carries has_more")
	require.False(t, *second.HasMore, "the feed ends on the second page")

	walked := make([]string, 0, 4)
	for _, item := range append(first.Items, second.Items...) {
		walked = append(walked, stringOf(item["id"]))
	}
	require.ElementsMatch(t, created, walked, "the two pages are the whole feed")
}

// TestListHandsItsAfterHookThePageItAnswers pins that the record a feed reads
// past the page, to tell whether records follow it, never reaches the
// service: ListAfter is handed the page the client is answered with.
func TestListHandsItsAfterHookThePageItAnswers(t *testing.T) {
	name := uniqueName("list-observed")
	for range 3 {
		createSample(t, name)
	}
	handler := controller.ListHandler[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](pageRoute))

	page := serveList(t, handler, "/"+pageRoute, "/"+pageRoute+"?name="+name+"&_size=2&_cursor_next=true")

	require.Len(t, page.Items, 2)
	require.NotNil(t, page.HasMore, "a list response carries has_more")
	require.True(t, *page.HasMore, "a third record follows the page")
	require.EqualValues(t, 2, observedPage.Load(), "the hook is handed the page, without the record read past it")
}

// TestListTellsWhenItsCapCutsAnUnpagedListShort pins has_more on a model that
// does not page, whose listing stops at 1000 records: false while every
// record fits, the cap filled exactly included, and true once one does not.
func TestListTellsWhenItsCapCutsAnUnpagedListShort(t *testing.T) {
	const limit = 1000
	ctx := context.Background()

	var stored int
	require.NoError(t, database.Database[*sampleCounter](ctx).Count(&stored))
	require.Less(t, stored, limit, "the counters the other tests leave fit under the cap")
	fill := make([]*sampleCounter, limit-stored)
	for i := range fill {
		fill[i] = &sampleCounter{Name: "list-cap"}
	}
	require.NoError(t, database.Database[*sampleCounter](ctx).Create(fill...))
	t.Cleanup(func() {
		require.NoError(t, database.Database[*sampleCounter](ctx).WithPurge().Delete(fill...))
	})
	handler := controller.ListHandler[*sampleCounter, *sampleCounter, *sampleCounter](configFor[*sampleCounter](counterRoute))

	filled := serveList(t, handler, "/"+counterRoute, "/"+counterRoute)
	require.Len(t, filled.Items, limit)
	require.Equal(t, limit, filled.Total)
	require.NotNil(t, filled.HasMore, "a list response carries has_more")
	require.False(t, *filled.HasMore, "the cap filled exactly leaves no record unread")

	past := &sampleCounter{Name: "list-cap"}
	require.NoError(t, database.Database[*sampleCounter](ctx).Create(past))
	t.Cleanup(func() {
		require.NoError(t, database.Database[*sampleCounter](ctx).WithPurge().Delete(past))
	})

	cut := serveList(t, handler, "/"+counterRoute, "/"+counterRoute)
	require.Len(t, cut.Items, limit)
	require.Equal(t, limit+1, cut.Total)
	require.NotNil(t, cut.HasMore, "a list response carries has_more")
	require.True(t, *cut.HasMore, "one record lies past the cap")
}

// listAnswer is the data of a list response; HasMore is nil when the
// response carries no has_more.
type listAnswer struct {
	Items   []map[string]any `json:"items"`
	Total   int              `json:"total"`
	HasMore *bool            `json:"has_more"`
}

// serveList sends handler, mounted under pattern, a listing of target, which it
// must answer 200, and returns the data of the response.
func serveList(t *testing.T, handler gin.HandlerFunc, pattern, target string) listAnswer {
	t.Helper()

	rsp := serve(t, http.MethodGet, pattern, handler, target, "")
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	var envelope struct {
		Data listAnswer `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &envelope))
	return envelope.Data
}
