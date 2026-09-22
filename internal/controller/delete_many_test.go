package controller_test

import (
	"net/http"
	"testing"

	"github.com/hydroan/gst/internal/controller"
	"github.com/stretchr/testify/require"
)

// TestDeleteManyKeepsTheRecordsTheBeforeHookRefuses pins that a
// DeleteManyBefore refusal answers with the hook's error and deletes
// nothing.
func TestDeleteManyKeepsTheRecordsTheBeforeHookRefuses(t *testing.T) {
	record := createSample(t, "delete-many-refused")

	rsp := serve(t, http.MethodDelete, "/controller-refusals/batch",
		controller.DeleteManyFactory[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](refusalRoute)),
		"/controller-refusals/batch", `{"ids":["`+record.GetID()+`"]}`)

	require.Equal(t, http.StatusConflict, rsp.Code)
	require.Contains(t, rsp.Body.String(), refusedMsg)
	requireSampleName(t, record.GetID(), "delete-many-refused")
}

// TestDeleteManyRefusesAnEmptyID pins the 400 of a batch delete listing an
// empty id: the request is defective, and the record beside it is not
// deleted either.
func TestDeleteManyRefusesAnEmptyID(t *testing.T) {
	record := createSample(t, "delete-many-identified")

	rsp := serve(t, http.MethodDelete, "/controller-samples/batch",
		controller.DeleteManyFactory[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](sampleRoute)),
		"/controller-samples/batch", `{"ids":["`+record.GetID()+`",""]}`)

	require.Equal(t, http.StatusBadRequest, rsp.Code)
	require.Contains(t, rsp.Body.String(), `"code":1000`)
	requireSampleName(t, record.GetID(), "delete-many-identified")
}
