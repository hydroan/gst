package controller_test

import (
	"net/http"
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
			controller.GetFactory[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](sampleRoute)),
			"/controller-samples/missing", "")

		require.Equal(t, http.StatusNotFound, rsp.Code)
	})

	t.Run("an id the integer key cannot hold", func(t *testing.T) {
		rsp := serve(t, http.MethodGet, "/controller-counters/:id",
			controller.GetFactory[*sampleCounter, *sampleCounter, *sampleCounter](configFor[*sampleCounter](counterRoute)),
			"/controller-counters/first", "")

		require.Equal(t, http.StatusNotFound, rsp.Code)
	})
}
