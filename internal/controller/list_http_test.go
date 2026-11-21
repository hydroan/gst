package controller_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/controller"
	"github.com/stretchr/testify/require"
)

// TestListRefusesMalformedQueryParameters pins the 400 of a listing whose
// query parameters the model cannot answer, each refused before any row is
// read: a parameter of a capability the model lacks, a parameter of the way
// of paging the list does not page by, and a column or an operator no such
// parameter takes.
func TestListRefusesMalformedQueryParameters(t *testing.T) {
	samples := controller.ListHandler[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](sampleRoute))
	counters := controller.ListHandler[*sampleCounter, *sampleCounter, *sampleCounter](configFor[*sampleCounter](counterRoute))
	feeds := controller.ListHandler[*feedSample, *feedSample, *feedSample](configFor[*feedSample](feedRoute))

	for _, tt := range []struct {
		name    string
		handler gin.HandlerFunc
		target  string
	}{
		{name: "an order on a model that does not sort", handler: counters, target: "/controller-counters?_sort_by=name"},
		{name: "a filter on a field the model lacks", handler: samples, target: "/controller-samples?missing[eq]=value"},
		{name: "a filter operator that does not exist", handler: samples, target: "/controller-samples?name[bogus]=value"},
		{name: "a sort column the model lacks", handler: samples, target: "/controller-samples?_sort_by=missing"},
		{name: "a sort direction that does not exist", handler: samples, target: "/controller-samples?_sort_by=name+sideways"},
		{name: "a cursor on a list that pages by number", handler: samples, target: "/controller-samples?_cursor_next=true"},
		{name: "a cursor beside a page number", handler: samples, target: "/controller-samples?_page=2&_cursor_value=value"},
		{name: "a page number on a feed", handler: feeds, target: "/controller-feed-samples?_page=2"},
		{name: "an order on a feed", handler: feeds, target: "/controller-feed-samples?_sort_by=name"},
		{name: "a cursor column the model lacks", handler: feeds, target: "/controller-feed-samples?_cursor_field=missing&_cursor_value=value"},
		{name: "a cursor column the model lacks on the first page", handler: feeds, target: "/controller-feed-samples?_cursor_field=missing"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pattern, _, _ := strings.Cut(tt.target, "?")
			rsp := serve(t, http.MethodGet, pattern, tt.handler, tt.target, "")

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
// feed, the list of a model embedding Cursor: four records read two a page
// take two requests, the first naming no cursor and the second page telling
// that none follow it, and no third request for an empty page.
func TestListWalksAFeedToItsEnd(t *testing.T) {
	name := uniqueName("list-feed")
	created := make([]string, 0, 4)
	for range 4 {
		created = append(created, createFeed(t, name).GetID())
	}
	handler := controller.ListHandler[*feedSample, *feedSample, *feedSample](configFor[*feedSample](feedRoute))

	first := serveList(t, handler, "/"+feedRoute, "/"+feedRoute+"?name="+name+"&_size=2")
	require.Len(t, first.Items, 2)
	require.Zero(t, first.Total, "a feed counts nothing")
	require.NotNil(t, first.HasMore, "a list response carries has_more")
	require.True(t, *first.HasMore, "two records follow the first page")

	last := stringOf(first.Items[1]["id"])
	second := serveList(t, handler, "/"+feedRoute, "/"+feedRoute+"?name="+name+"&_size=2&_cursor_next=true&_cursor_value="+last)
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
		createFeed(t, name)
	}
	handler := controller.ListHandler[*feedSample, *feedSample, *feedSample](configFor[*feedSample](pageRoute))

	page := serveList(t, handler, "/"+pageRoute, "/"+pageRoute+"?name="+name+"&_size=2")

	require.Len(t, page.Items, 2)
	require.NotNil(t, page.HasMore, "a list response carries has_more")
	require.True(t, *page.HasMore, "a third record follows the page")
	require.EqualValues(t, 2, observedPage.Load(), "the hook is handed the page, without the record read past it")
}

// TestListPagesAModelThatEmbedsNothing pins that a list pages with nothing
// embedded in its model: a request naming no size is answered the default
// page of 20 records with whether records follow it, and _page reads on.
func TestListPagesAModelThatEmbedsNothing(t *testing.T) {
	ctx := context.Background()
	created := make([]*sampleCounter, 21)
	for i := range created {
		created[i] = &sampleCounter{Name: "list-paged"}
	}
	require.NoError(t, database.Database[*sampleCounter](ctx).Create(created...))
	t.Cleanup(func() {
		require.NoError(t, database.Database[*sampleCounter](ctx).WithPurge().Delete(created...))
	})
	handler := controller.ListHandler[*sampleCounter, *sampleCounter, *sampleCounter](configFor[*sampleCounter](counterRoute))

	first := serveList(t, handler, "/"+counterRoute, "/"+counterRoute)
	require.Len(t, first.Items, 20, "a size left out is the default page size")
	require.GreaterOrEqual(t, first.Total, 21)
	require.NotNil(t, first.HasMore, "a list response carries has_more")
	require.True(t, *first.HasMore, "a twenty-first record follows the page")

	second := serveList(t, handler, "/"+counterRoute, "/"+counterRoute+"?_page=2&_size=20")
	require.NotEmpty(t, second.Items, "the second page reads on")
	require.NotEqual(t, first.Items[0]["id"], second.Items[0]["id"], "the second page starts past the first")
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
