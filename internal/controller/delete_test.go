package controller_test

import (
	"net/http"
	"testing"

	"github.com/hydroan/gst/internal/controller"
	"github.com/stretchr/testify/require"
)

// TestDeleteAnswersNotFoundForAnIDTheIntegerKeyCannotHold pins that a delete
// whose id an integer key cannot hold answers 404 without reaching SQL.
func TestDeleteAnswersNotFoundForAnIDTheIntegerKeyCannotHold(t *testing.T) {
	rsp := serve(t, http.MethodDelete, "/controller-counters/:id",
		controller.DeleteFactory[*sampleCounter, *sampleCounter, *sampleCounter](configFor[*sampleCounter](counterRoute)),
		"/controller-counters/first", "")

	require.Equal(t, http.StatusNotFound, rsp.Code)
}

// TestDeleteKeepsTheRecordTheBeforeHookRefuses pins that a DeleteBefore
// refusal answers with the hook's error and deletes nothing.
func TestDeleteKeepsTheRecordTheBeforeHookRefuses(t *testing.T) {
	record := createSample(t, "delete-refused")

	rsp := serve(t, http.MethodDelete, "/controller-refusals/:id",
		controller.DeleteFactory[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](refusalRoute)),
		"/controller-refusals/"+record.GetID(), "")

	require.Equal(t, http.StatusConflict, rsp.Code)
	require.Contains(t, rsp.Body.String(), refusedMsg)
	requireSampleName(t, record.GetID(), "delete-refused")
}
