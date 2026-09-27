package controller_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/controller"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
)

// TestPatchItemReadiesTheRecordOfABatchItem pins PatchItem, what the
// generated handler of a PatchMany rpc reads each item through: the item
// names its record by id, which a record carrying no id takes and one
// carrying the same id keeps; a record naming another id, an item naming no
// id and an item whose route parameter contradicts the request's are
// refused with InvalidArgument; a parameter the item leaves empty is the
// request's; and an item carrying no record is left to the call to refuse.
func TestPatchItemReadiesTheRecordOfABatchItem(t *testing.T) {
	readied, err := controller.PatchItem(0, nil, nil, "r1", &sampleRecord{Name: "named"})
	require.NoError(t, err)
	require.Equal(t, "r1", readied.GetID())
	require.Equal(t, "named", readied.Name)

	same := &sampleRecord{}
	same.SetID("r1")
	readied, err = controller.PatchItem(0, nil, nil, "r1", same)
	require.NoError(t, err)
	require.Equal(t, "r1", readied.GetID())

	other := &sampleRecord{}
	other.SetID("r2")
	_, err = controller.PatchItem(1, nil, nil, "r1", other)
	requireStatus(t, err, codes.InvalidArgument, "item 1 names the record r1 but carries the record r2")

	_, err = controller.PatchItem(2, nil, nil, "", &sampleRecord{})
	requireStatus(t, err, codes.InvalidArgument, "item 2 names no id")

	_, err = controller.PatchItem(3, map[string]string{"record": "a"}, map[string]string{"record": "b"}, "r1", &sampleRecord{})
	requireStatus(t, err, codes.InvalidArgument, `item 3 names the record parameter "b", the request names "a"`)

	readied, err = controller.PatchItem(4, map[string]string{"record": "a"}, map[string]string{"record": ""}, "r1", &sampleRecord{})
	require.NoError(t, err)
	require.Equal(t, "r1", readied.GetID())

	var absent *sampleRecord
	readied, err = controller.PatchItem(5, nil, nil, "r1", absent)
	require.NoError(t, err)
	require.Nil(t, readied)
}

// TestPatchManyReportsAMissingVersionBeforeAMissingRecord pins which failure
// a batch patch of a versioned model reports for an item that carries no
// version and names a record that does not exist: the missing version, 400,
// because every item's version is checked before any record is loaded.
// Loading the record first would answer 404 instead.
func TestPatchManyReportsAMissingVersionBeforeAMissingRecord(t *testing.T) {
	rsp := serve(t, http.MethodPatch, "/controller-versioned-samples/batch",
		controller.PatchManyHandler[*versionedSample, *versionedSample, *versionedSample](),
		"/controller-versioned-samples/batch", `{"items":[{"id":"missing","name":"renamed"}]}`)

	require.Equal(t, http.StatusBadRequest, rsp.Code)
	require.Contains(t, rsp.Body.String(), `"code":1000`)
}

// TestPatchManyValidatesTheFieldsEachItemNames pins that a batch patch
// checks each item against the binding tags of the fields it names and no
// other: an item leaving the required name out passes, one naming it empty
// refuses the batch.
func TestPatchManyValidatesTheFieldsEachItemNames(t *testing.T) {
	record := &validatedSample{Name: "validated-batch-patch"}
	require.NoError(t, database.Database[*validatedSample](context.Background()).Create(record))
	handler := controller.PatchManyHandler[*validatedSample, *validatedSample, *validatedSample](configFor[*validatedSample](validatedRoute))

	rsp := serve(t, http.MethodPatch, "/controller-validated-samples/batch", handler, "/controller-validated-samples/batch", `{"items":[{"id":"`+record.GetID()+`","note":"only the note"}]}`)
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())

	rsp = serve(t, http.MethodPatch, "/controller-validated-samples/batch", handler, "/controller-validated-samples/batch", `{"items":[{"id":"`+record.GetID()+`","name":""}]}`)
	require.Equal(t, http.StatusBadRequest, rsp.Code)
}

// TestPatchManyDropsANullItemWithItsFieldSet pins that a null item of a
// batch patch is dropped together with its field set: the item after it is
// patched on the fields it names itself, not on the null's empty set.
func TestPatchManyDropsANullItemWithItsFieldSet(t *testing.T) {
	record := createSample(t, "patch-many-after-null")

	rsp := serve(t, http.MethodPatch, "/controller-samples/batch",
		controller.PatchManyHandler[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](sampleRoute)),
		"/controller-samples/batch", `{"items":[null,{"id":"`+record.GetID()+`","name":"patch-many-renamed-after-null"}]}`)

	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	requireSampleName(t, record.GetID(), "patch-many-renamed-after-null")
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
				controller.PatchManyHandler[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](sampleRoute)),
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
		controller.PatchManyHandler[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](sampleRoute)),
		"/controller-samples/batch", `{"items":[{"id":"`+record.GetID()+`","name":"patch-many-renamed"},{"name":"patch-many-other"}]}`)

	require.Equal(t, http.StatusBadRequest, rsp.Code)
	require.Contains(t, rsp.Body.String(), `"code":1000`)
	requireSampleName(t, record.GetID(), "patch-many-identified")
}
