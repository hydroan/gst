package controller_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/controller"
	"github.com/stretchr/testify/require"
)

// TestPatchAnswersNotFound pins the 404 of a patch whose id names no record:
// an id no row carries, and one an integer key cannot hold, which is refused
// before it reaches SQL.
func TestPatchAnswersNotFound(t *testing.T) {
	t.Run("an id no record carries", func(t *testing.T) {
		rsp := serve(t, http.MethodPatch, "/controller-samples/:id",
			controller.PatchHandler[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](sampleRoute)),
			"/controller-samples/missing", `{"name":"renamed"}`)

		require.Equal(t, http.StatusNotFound, rsp.Code)
	})

	t.Run("an id the integer key cannot hold", func(t *testing.T) {
		rsp := serve(t, http.MethodPatch, "/controller-counters/:id",
			controller.PatchHandler[*sampleCounter, *sampleCounter, *sampleCounter](configFor[*sampleCounter](counterRoute)),
			"/controller-counters/first", `{"name":"renamed"}`)

		require.Equal(t, http.StatusNotFound, rsp.Code)
	})
}

// TestPatchRefusesAVersionedRecordWithoutItsVersion pins the 400 of a patch
// of a versioned model that carries no version: without it the lock would
// check the row against itself.
func TestPatchRefusesAVersionedRecordWithoutItsVersion(t *testing.T) {
	rsp := serve(t, http.MethodPatch, "/controller-versioned-samples/:id",
		controller.PatchHandler[*versionedSample, *versionedSample, *versionedSample](configFor[*versionedSample](versionedRoute)),
		"/controller-versioned-samples/missing", `{"name":"renamed"}`)

	require.Equal(t, http.StatusBadRequest, rsp.Code)
}

// TestPatchValidatesTheFieldsTheBodyNames pins that a patch is checked
// against the binding tags of the fields its body names and no other: a body
// leaving the required name out passes, one naming it empty is refused.
func TestPatchValidatesTheFieldsTheBodyNames(t *testing.T) {
	record := &validatedSample{Name: "validated-patch"}
	require.NoError(t, database.Database[*validatedSample](context.Background()).Create(record))
	handler := controller.PatchHandler[*validatedSample, *validatedSample, *validatedSample](configFor[*validatedSample](validatedRoute))

	rsp := serve(t, http.MethodPatch, "/controller-validated-samples/:id", handler, "/controller-validated-samples/"+record.GetID(), `{"note":"only the note"}`)
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())

	rsp = serve(t, http.MethodPatch, "/controller-validated-samples/:id", handler, "/controller-validated-samples/"+record.GetID(), `{"name":""}`)
	require.Equal(t, http.StatusBadRequest, rsp.Code)
}

// TestPatchKeepsTheRecordTheBeforeHookRefuses pins that a PatchBefore
// refusal answers with the hook's error and writes nothing.
func TestPatchKeepsTheRecordTheBeforeHookRefuses(t *testing.T) {
	record := createSample(t, "patch-refused")

	rsp := serve(t, http.MethodPatch, "/controller-refusals/:id",
		controller.PatchHandler[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](refusalRoute)),
		"/controller-refusals/"+record.GetID(), `{"name":"renamed"}`)

	require.Equal(t, http.StatusConflict, rsp.Code)
	require.Contains(t, rsp.Body.String(), refusedMsg)
	requireSampleName(t, record.GetID(), "patch-refused")
}
