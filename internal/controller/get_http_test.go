package controller_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hydroan/gst/internal/controller"
	"github.com/stretchr/testify/require"
)

// TestGetAnswersNotFound pins the 404 of a get whose id names no record: an
// id no row carries, and one an integer key cannot hold, which is refused
// before it reaches SQL.
func TestGetAnswersNotFound(t *testing.T) {
	t.Run("an id no record carries", func(t *testing.T) {
		rsp := serve(t, http.MethodGet, "/controller-samples/:id",
			controller.GetHandler[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](sampleRoute)),
			"/controller-samples/missing", "")

		require.Equal(t, http.StatusNotFound, rsp.Code)
	})

	t.Run("an id the integer key cannot hold", func(t *testing.T) {
		rsp := serve(t, http.MethodGet, "/controller-counters/:id",
			controller.GetHandler[*sampleCounter, *sampleCounter, *sampleCounter](configFor[*sampleCounter](counterRoute)),
			"/controller-counters/first", "")

		require.Equal(t, http.StatusNotFound, rsp.Code)
	})
}

// TestGetRefusesADepthThatIsNoNumber pins that a get, which decodes no model
// from the query, refuses a _depth that is no number itself, in the words a
// list answers one its model cannot decode; a depth of zero is the depth
// left out.
func TestGetRefusesADepthThatIsNoNumber(t *testing.T) {
	id := createSample(t, "get-depth").GetID()
	get := func(query string) *httptest.ResponseRecorder {
		return serve(t, http.MethodGet, "/controller-samples/:id",
			controller.GetHandler[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](sampleRoute)),
			"/controller-samples/"+id+"?"+query, "")
	}

	for _, depth := range []string{"abc", "-1", "1.5"} {
		rsp := get("_depth=" + depth)
		require.Equal(t, http.StatusBadRequest, rsp.Code, rsp.Body.String())
		require.Contains(t, rsp.Body.String(), `"msg":"invalid query parameter \"_depth\": expect a numeric value, got \"`+depth+`\""`)
	}

	rsp := get("_depth=0")
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
}
