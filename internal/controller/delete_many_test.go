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
