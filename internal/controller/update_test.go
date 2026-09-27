package controller_test

import (
	"net/http"
	"testing"

	"github.com/hydroan/gst/internal/controller"
	"github.com/stretchr/testify/require"
)

// TestUpdateAnswersNotFound pins the 404 of an update whose id names no
// record: an id no row carries, which the database layer reports, and one an
// integer key cannot hold, which is refused before it reaches SQL.
func TestUpdateAnswersNotFound(t *testing.T) {
	t.Run("an id no record carries", func(t *testing.T) {
		rsp := serve(t, http.MethodPut, "/controller-samples/:id",
			controller.UpdateHandler[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](sampleRoute)),
			"/controller-samples/missing", `{"name":"renamed"}`)

		require.Equal(t, http.StatusNotFound, rsp.Code)
	})

	t.Run("an id the integer key cannot hold", func(t *testing.T) {
		rsp := serve(t, http.MethodPut, "/controller-counters/:id",
			controller.UpdateHandler[*sampleCounter, *sampleCounter, *sampleCounter](configFor[*sampleCounter](counterRoute)),
			"/controller-counters/first", `{"name":"renamed"}`)

		require.Equal(t, http.StatusNotFound, rsp.Code)
	})
}

// TestUpdateWritesTheRecordTheRouteNames pins that the id of the route names
// the record an update replaces: an id the body carries moves the update to
// no other record, and the record answered carries the route's id.
func TestUpdateWritesTheRecordTheRouteNames(t *testing.T) {
	target := createSample(t, "update-target")
	other := createSample(t, "update-other")

	rsp := serve(t, http.MethodPut, "/controller-samples/:id",
		controller.UpdateHandler[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](sampleRoute)),
		"/controller-samples/"+target.GetID(), `{"id":"`+other.GetID()+`","name":"update-renamed"}`)

	require.Equal(t, http.StatusOK, rsp.Code)
	require.Contains(t, rsp.Body.String(), `"id":"`+target.GetID()+`"`)
	requireSampleName(t, target.GetID(), "update-renamed")
	requireSampleName(t, other.GetID(), "update-other")
}

// TestUpdateKeepsTheRecordTheBeforeHookRefuses pins that an UpdateBefore
// refusal answers with the hook's error and writes nothing.
func TestUpdateKeepsTheRecordTheBeforeHookRefuses(t *testing.T) {
	record := createSample(t, "update-refused")

	rsp := serve(t, http.MethodPut, "/controller-refusals/:id",
		controller.UpdateHandler[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](refusalRoute)),
		"/controller-refusals/"+record.GetID(), `{"name":"renamed"}`)

	require.Equal(t, http.StatusConflict, rsp.Code)
	require.Contains(t, rsp.Body.String(), refusedMsg)
	requireSampleName(t, record.GetID(), "update-refused")
}
