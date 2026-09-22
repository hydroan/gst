package controller_test

import (
	"net/http"
	"testing"

	"github.com/hydroan/gst/internal/controller"
	"github.com/stretchr/testify/require"
)

// TestUpdateManyWritesNothingWhenOneRecordIsMissing pins that a batch update
// is all or nothing: an item naming a record that does not exist fails the
// batch with 404, and the record beside it keeps what it stored.
func TestUpdateManyWritesNothingWhenOneRecordIsMissing(t *testing.T) {
	record := createSample(t, "update-many-kept")

	rsp := serve(t, http.MethodPut, "/controller-samples/batch",
		controller.UpdateManyFactory[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](sampleRoute)),
		"/controller-samples/batch", `{"items":[{"id":"`+record.GetID()+`","name":"update-many-renamed"},{"id":"missing","name":"update-many-other"}]}`)

	require.Equal(t, http.StatusNotFound, rsp.Code)
	requireSampleName(t, record.GetID(), "update-many-kept")
}

// TestUpdateManyWritesNothingTheBeforeHookRefuses pins that an
// UpdateManyBefore refusal answers with the hook's error and writes nothing.
func TestUpdateManyWritesNothingTheBeforeHookRefuses(t *testing.T) {
	record := createSample(t, "update-many-refused")

	rsp := serve(t, http.MethodPut, "/controller-refusals/batch",
		controller.UpdateManyFactory[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](refusalRoute)),
		"/controller-refusals/batch", `{"items":[{"id":"`+record.GetID()+`","name":"update-many-renamed"}]}`)

	require.Equal(t, http.StatusConflict, rsp.Code)
	require.Contains(t, rsp.Body.String(), refusedMsg)
	requireSampleName(t, record.GetID(), "update-many-refused")
}

// TestUpdateManyRefusesAnItemWithoutAnID pins the 400 of a batch update
// with an item that names no record: the request is defective, and the item
// beside it is not written either.
func TestUpdateManyRefusesAnItemWithoutAnID(t *testing.T) {
	record := createSample(t, "update-many-identified")

	rsp := serve(t, http.MethodPut, "/controller-samples/batch",
		controller.UpdateManyFactory[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](sampleRoute)),
		"/controller-samples/batch", `{"items":[{"id":"`+record.GetID()+`","name":"update-many-renamed"},{"name":"update-many-other"}]}`)

	require.Equal(t, http.StatusBadRequest, rsp.Code)
	require.Contains(t, rsp.Body.String(), `"code":1000`)
	requireSampleName(t, record.GetID(), "update-many-identified")
}
