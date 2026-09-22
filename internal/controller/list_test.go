package controller_test

import (
	"net/http"
	"testing"

	"github.com/hydroan/gst/internal/controller"
	"github.com/stretchr/testify/require"
)

// TestListRefusesMalformedQueryParameters pins the 400 of a listing whose
// query parameters the model cannot answer, each refused before any row is
// read.
func TestListRefusesMalformedQueryParameters(t *testing.T) {
	samples := controller.ListFactory[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](sampleRoute))
	counters := controller.ListFactory[*sampleCounter, *sampleCounter, *sampleCounter](configFor[*sampleCounter](counterRoute))

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
		{name: "a cursor beside an order of its own", target: "/controller-samples?_cursor_field=id&_cursor_value=value&_sort_by=name"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			handler, pattern := samples, "/controller-samples"
			if tt.counts {
				handler, pattern = counters, "/controller-counters"
			}
			rsp := serve(t, http.MethodGet, pattern, handler, tt.target, "")

			require.Equal(t, http.StatusBadRequest, rsp.Code, rsp.Body.String())
			require.Contains(t, rsp.Body.String(), `"code":1000`)
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
				controller.ListFactory[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](route)),
				"/"+route, "")

			require.Equal(t, http.StatusConflict, rsp.Code)
			require.Contains(t, rsp.Body.String(), refusedMsg)
			require.NotContains(t, rsp.Body.String(), name)
		})
	}
}
