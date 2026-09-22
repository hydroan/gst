package controller_test

import (
	"net/http"
	"testing"

	"github.com/hydroan/gst/internal/controller"
	"github.com/stretchr/testify/require"
)

// TestUpdateManyWritesNothingWhenOneRecordIsMissing pins that a batch update
// is all or nothing: an item naming a record that does not exist — an unknown
// id, or one of whitespace alone, used as sent — fails the batch with 404,
// and the record beside it keeps what it stored.
func TestUpdateManyWritesNothingWhenOneRecordIsMissing(t *testing.T) {
	tests := []struct {
		name string
		id   string // the id as written inside the JSON string
	}{
		{name: "unknown", id: `missing`},
		{name: "blank", id: ` \t `},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := createSample(t, "update-many-kept-"+tt.name)

			rsp := serve(t, http.MethodPut, "/controller-samples/batch",
				controller.UpdateManyFactory[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](sampleRoute)),
				"/controller-samples/batch", `{"items":[{"id":"`+record.GetID()+`","name":"update-many-renamed"},{"id":"`+tt.id+`","name":"update-many-other"}]}`)

			require.Equal(t, http.StatusNotFound, rsp.Code)
			requireSampleName(t, record.GetID(), "update-many-kept-"+tt.name)
		})
	}
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
