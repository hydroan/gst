package controller_test

import (
	"net/http"
	"testing"

	"github.com/hydroan/gst/internal/controller"
	"github.com/stretchr/testify/require"
)

// TestCreateManyWritesNothingWhenOneItemCollides pins that a batch create is
// all or nothing: an item whose id a stored record already carries fails the
// batch with 409, and the item beside it is not created either.
func TestCreateManyWritesNothingWhenOneItemCollides(t *testing.T) {
	record := createSample(t, "create-many-taken")
	fresh := uniqueName("create-many-fresh")

	rsp := serve(t, http.MethodPost, "/controller-samples/batch",
		controller.CreateManyFactory[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](sampleRoute)),
		"/controller-samples/batch", `{"items":[{"name":"`+fresh+`"},{"id":"`+record.GetID()+`","name":"create-many-other"}]}`)

	require.Equal(t, http.StatusConflict, rsp.Code)
	require.Zero(t, countSamplesNamed(t, fresh))
	requireSampleName(t, record.GetID(), "create-many-taken")
}

// TestCreateManyWritesNothingTheBeforeHookRefuses pins that a
// CreateManyBefore refusal answers with the hook's error and creates
// nothing.
func TestCreateManyWritesNothingTheBeforeHookRefuses(t *testing.T) {
	name := uniqueName("create-many-refused")

	rsp := serve(t, http.MethodPost, "/controller-refusals/batch",
		controller.CreateManyFactory[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](refusalRoute)),
		"/controller-refusals/batch", `{"items":[{"name":"`+name+`"}]}`)

	require.Equal(t, http.StatusConflict, rsp.Code)
	require.Contains(t, rsp.Body.String(), refusedMsg)
	require.Zero(t, countSamplesNamed(t, name))
}

// TestCreateManyIgnoresMembersTheBatchDoesNotDeclare pins that a batch
// request carrying a member the batch body does not declare, such as the
// options member it once had, is applied as if the member were absent.
func TestCreateManyIgnoresMembersTheBatchDoesNotDeclare(t *testing.T) {
	name := uniqueName("create-many-optioned")

	rsp := serve(t, http.MethodPost, "/controller-samples/batch",
		controller.CreateManyFactory[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](sampleRoute)),
		"/controller-samples/batch", `{"items":[{"name":"`+name+`"}],"options":{"atomic":true}}`)

	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	require.Equal(t, 1, countSamplesNamed(t, name))
}
