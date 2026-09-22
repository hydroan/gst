package controller_test

import (
	"net/http"
	"testing"

	"github.com/hydroan/gst/internal/controller"
	"github.com/stretchr/testify/require"
)

// TestPatchAnswersNotFound pins the 404 of a patch whose id names no record:
// an id no row carries, and one an integer key cannot hold, which is refused
// before it reaches SQL.
func TestPatchAnswersNotFound(t *testing.T) {
	t.Run("an id no record carries", func(t *testing.T) {
		rsp := serve(t, http.MethodPatch, "/controller-samples/:id",
			controller.PatchFactory[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](sampleRoute)),
			"/controller-samples/missing", `{"name":"renamed"}`)

		require.Equal(t, http.StatusNotFound, rsp.Code)
	})

	t.Run("an id the integer key cannot hold", func(t *testing.T) {
		rsp := serve(t, http.MethodPatch, "/controller-counters/:id",
			controller.PatchFactory[*sampleCounter, *sampleCounter, *sampleCounter](configFor[*sampleCounter](counterRoute)),
			"/controller-counters/first", `{"name":"renamed"}`)

		require.Equal(t, http.StatusNotFound, rsp.Code)
	})
}

// TestPatchRefusesAVersionedRecordWithoutItsVersion pins the 400 of a patch
// of a versioned model that carries no version: without it the lock would
// check the row against itself.
func TestPatchRefusesAVersionedRecordWithoutItsVersion(t *testing.T) {
	rsp := serve(t, http.MethodPatch, "/controller-versioned-samples/:id",
		controller.PatchFactory[*versionedSample, *versionedSample, *versionedSample](configFor[*versionedSample](versionedRoute)),
		"/controller-versioned-samples/missing", `{"name":"renamed"}`)

	require.Equal(t, http.StatusBadRequest, rsp.Code)
	require.Contains(t, rsp.Body.String(), `"code":1000`)
}

// TestPatchKeepsTheRecordTheBeforeHookRefuses pins that a PatchBefore
// refusal answers with the hook's error and writes nothing.
func TestPatchKeepsTheRecordTheBeforeHookRefuses(t *testing.T) {
	record := createSample(t, "patch-refused")

	rsp := serve(t, http.MethodPatch, "/controller-refusals/:id",
		controller.PatchFactory[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](refusalRoute)),
		"/controller-refusals/"+record.GetID(), `{"name":"renamed"}`)

	require.Equal(t, http.StatusConflict, rsp.Code)
	require.Contains(t, rsp.Body.String(), refusedMsg)
	requireSampleName(t, record.GetID(), "patch-refused")
}
