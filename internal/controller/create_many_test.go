package controller_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/hydroan/gst/database"
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
		controller.CreateManyHandler[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](sampleRoute)),
		"/controller-samples/batch", `{"items":[{"name":"`+fresh+`"},{"id":"`+record.GetID()+`","name":"create-many-other"}]}`)

	require.Equal(t, http.StatusConflict, rsp.Code)
	require.Zero(t, countSamplesNamed(t, fresh))
	requireSampleName(t, record.GetID(), "create-many-taken")
}

// TestCreateManyRefusesAnItemFailingValidation pins that the items of a
// batch are validated against their binding tags the way a single
// resource's body is: the validator descends into the items, so an item
// without its required field refuses the whole batch before anything is
// written. The items are validated as the service would receive them, the
// null entries dropped first, so the refusal names the item by the position
// it holds in the batch the service sees, the way the gRPC call does.
func TestCreateManyRefusesAnItemFailingValidation(t *testing.T) {
	rsp := serve(t, http.MethodPost, "/controller-validated-samples/batch",
		controller.CreateManyHandler[*validatedSample, *validatedSample, *validatedSample](configFor[*validatedSample](validatedRoute)),
		"/controller-validated-samples/batch", `{"items":[{"name":"valid"},{}]}`)

	require.Equal(t, http.StatusBadRequest, rsp.Code)
	require.Contains(t, rsp.Body.String(), `"msg":"items[1].name is a required field"`)
	var total int
	require.NoError(t, database.Database[*validatedSample](context.Background()).WithQuery(&validatedSample{Name: "valid"}).Count(&total))
	require.Zero(t, total)

	rsp = serve(t, http.MethodPost, "/controller-validated-samples/batch",
		controller.CreateManyHandler[*validatedSample, *validatedSample, *validatedSample](configFor[*validatedSample](validatedRoute)),
		"/controller-validated-samples/batch", `{"items":[null,{}]}`)

	require.Equal(t, http.StatusBadRequest, rsp.Code)
	require.Contains(t, rsp.Body.String(), `"msg":"items[0].name is a required field"`, "the null entry is dropped before the items are validated")
}

// TestCreateManyWritesNothingTheBeforeHookRefuses pins that a
// CreateManyBefore refusal answers with the hook's error and creates
// nothing.
func TestCreateManyWritesNothingTheBeforeHookRefuses(t *testing.T) {
	name := uniqueName("create-many-refused")

	rsp := serve(t, http.MethodPost, "/controller-refusals/batch",
		controller.CreateManyHandler[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](refusalRoute)),
		"/controller-refusals/batch", `{"items":[{"name":"`+name+`"}]}`)

	require.Equal(t, http.StatusConflict, rsp.Code)
	require.Contains(t, rsp.Body.String(), refusedMsg)
	require.Zero(t, countSamplesNamed(t, name))
}

// TestCreateManyIgnoresMembersTheBatchDoesNotDeclare pins that a batch
// request carrying a member the batch body does not declare, such as an
// options member, is applied as if the member were absent.
func TestCreateManyIgnoresMembersTheBatchDoesNotDeclare(t *testing.T) {
	name := uniqueName("create-many-optioned")

	rsp := serve(t, http.MethodPost, "/controller-samples/batch",
		controller.CreateManyHandler[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](sampleRoute)),
		"/controller-samples/batch", `{"items":[{"name":"`+name+`"}],"options":{"atomic":true}}`)

	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	require.Equal(t, 1, countSamplesNamed(t, name))
}
