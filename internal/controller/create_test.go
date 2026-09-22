package controller_test

import (
	"net/http"
	"testing"

	"github.com/hydroan/gst/internal/controller"
	"github.com/stretchr/testify/require"
)

// TestCreateWritesNothingTheBeforeHookRefuses pins that a CreateBefore
// refusal answers with the hook's error and creates nothing.
func TestCreateWritesNothingTheBeforeHookRefuses(t *testing.T) {
	name := uniqueName("create-refused")

	rsp := serve(t, http.MethodPost, "/controller-refusals",
		controller.CreateFactory[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](refusalRoute)),
		"/controller-refusals", `{"name":"`+name+`"}`)

	require.Equal(t, http.StatusConflict, rsp.Code)
	require.Contains(t, rsp.Body.String(), refusedMsg)
	require.Zero(t, countSamplesNamed(t, name))
}

// TestCreateAnswersConflictForATakenID pins the 409 of a create whose id a
// stored record already carries, and that the stored record stays as it was.
func TestCreateAnswersConflictForATakenID(t *testing.T) {
	record := createSample(t, "create-taken")

	rsp := serve(t, http.MethodPost, "/controller-samples",
		controller.CreateFactory[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](sampleRoute)),
		"/controller-samples", `{"id":"`+record.GetID()+`","name":"create-other"}`)

	require.Equal(t, http.StatusConflict, rsp.Code)
	requireSampleName(t, record.GetID(), "create-taken")
}
