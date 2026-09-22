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

// TestPatchManyWritesNothingWhenOneRecordIsMissing pins that a batch patch
// is all or nothing: an item naming a record that does not exist — an unknown
// id, or one of whitespace alone, used as sent — fails the batch with 404,
// and the record beside it keeps what it stored.
func TestPatchManyWritesNothingWhenOneRecordIsMissing(t *testing.T) {
	tests := []struct {
		name string
		id   string // the id as written inside the JSON string
	}{
		{name: "unknown", id: `missing`},
		{name: "blank", id: ` \t `},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := createSample(t, "patch-many-kept-"+tt.name)

			rsp := serve(t, http.MethodPatch, "/controller-samples/batch",
				controller.PatchManyFactory[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](sampleRoute)),
				"/controller-samples/batch", `{"items":[{"id":"`+record.GetID()+`","name":"patch-many-renamed"},{"id":"`+tt.id+`","name":"patch-many-other"}]}`)

			require.Equal(t, http.StatusNotFound, rsp.Code)
			requireSampleName(t, record.GetID(), "patch-many-kept-"+tt.name)
		})
	}
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
