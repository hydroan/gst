package controller_test

import (
	"net/http"
	"testing"

	"github.com/hydroan/gst/internal/controller"
	"github.com/stretchr/testify/require"
)

// TestPatchManyReportsAMissingVersionBeforeAMissingRecord pins which failure
// a batch patch of a versioned model reports for an item that carries no
// version and names a record that does not exist: the missing version, 400,
// because every item's version is checked before any record is loaded.
// Loading the record first would answer 404 instead.
func TestPatchManyReportsAMissingVersionBeforeAMissingRecord(t *testing.T) {
	rsp := serve(t, http.MethodPatch, "/controller-versioned-samples/batch",
		controller.PatchManyFactory[*versionedSample, *versionedSample, *versionedSample](),
		"/controller-versioned-samples/batch", `{"items":[{"id":"missing","name":"renamed"}]}`)

	require.Equal(t, http.StatusBadRequest, rsp.Code)
	require.Contains(t, rsp.Body.String(), `"code":1000`)
}

// TestPatchManyRefusesAnItemWithoutAnID pins the 400 of a batch patch with
// an item that names no record: the request is defective, and the item
// beside it is not written either.
func TestPatchManyRefusesAnItemWithoutAnID(t *testing.T) {
	record := createSample(t, "patch-many-identified")

	rsp := serve(t, http.MethodPatch, "/controller-samples/batch",
		controller.PatchManyFactory[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](sampleRoute)),
		"/controller-samples/batch", `{"items":[{"id":"`+record.GetID()+`","name":"patch-many-renamed"},{"name":"patch-many-other"}]}`)

	require.Equal(t, http.StatusBadRequest, rsp.Code)
	require.Contains(t, rsp.Body.String(), `"code":1000`)
	requireSampleName(t, record.GetID(), "patch-many-identified")
}
