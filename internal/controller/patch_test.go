package controller_test

import (
	"context"
	"net/http"
	"testing"
	"time"

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

// TestPatchAppliesAFieldAsAWhole pins what a body key patches: a time
// value and a struct value are replaced as a whole, the parts of the struct
// the body left out included, a key promoted from an embedded struct sets
// its field while the neighbor stays, a struct the body names is checked
// against its own binding tags as a whole, and null on a value clears it.
func TestPatchAppliesAFieldAsAWhole(t *testing.T) {
	record := createShaped(t, "patch-shaped")
	handler := controller.PatchHandler[*shapedSample, *shapedSample, *shapedSample](configFor[*shapedSample](shapedRoute))
	target := "/controller-shaped-samples/" + record.GetID()
	dueAt := time.Date(2027, 1, 2, 3, 4, 5, 0, time.UTC)

	rsp := serve(t, http.MethodPatch, "/controller-shaped-samples/:id", handler, target, `{"due_at":"2027-01-02T03:04:05Z","address":{"city":"new"},"reviewer":"second"}`)
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	stored := loadShaped(t, record.GetID())
	require.Equal(t, "patch-shaped", stored.Name)
	require.True(t, stored.DueAt.Equal(dueAt), stored.DueAt)
	require.Equal(t, sampleAddress{City: "new"}, stored.Address, "a struct value is replaced as a whole")
	require.Equal(t, SampleAudit{Reviewer: "second", Reviewed: true}, stored.SampleAudit)

	rsp = serve(t, http.MethodPatch, "/controller-shaped-samples/:id", handler, target, `{"address":{"zip":"2"}}`)
	require.Equal(t, http.StatusBadRequest, rsp.Code, "the address is checked as a whole, its city required")
	require.Equal(t, sampleAddress{City: "new"}, loadShaped(t, record.GetID()).Address)

	rsp = serve(t, http.MethodPatch, "/controller-shaped-samples/:id", handler, target, `{"due_at":null}`)
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	require.True(t, loadShaped(t, record.GetID()).DueAt.IsZero(), "null clears a value")
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
