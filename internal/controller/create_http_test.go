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
		controller.CreateHandler[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](refusalRoute)),
		"/controller-refusals", `{"name":"`+name+`"}`)

	require.Equal(t, http.StatusConflict, rsp.Code)
	require.Contains(t, rsp.Body.String(), refusedMsg)
	require.Zero(t, countSamplesNamed(t, name))
}

// TestCreateReadsATimeOfDayAsWritten pins how a body's time of day is read:
// in time.TimeOnly, a fractional second as written, which the answer carries
// as the record the write took, whatever precision the column keeps; a value
// in another form or of another JSON type is refused with 400 naming the
// field, and nothing is stored.
func TestCreateReadsATimeOfDayAsWritten(t *testing.T) {
	handler := controller.CreateHandler[*datedSample, *datedSample, *datedSample](configFor[*datedSample](datedRoute))

	rsp := serve(t, http.MethodPost, "/controller-dated-samples", handler, "/controller-dated-samples", `{"name":"`+uniqueName("create-clock")+`","day":"2026-01-01T00:00:00Z","clock":"17:30:00.5"}`)
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	require.Contains(t, rsp.Body.String(), `"clock":"17:30:00.500000000"`)

	for form, value := range map[string]string{"malformed": `"abc"`, "number": `1730`, "out_of_range": `"25:00:00"`} {
		name := uniqueName("create-clock-" + form)
		rsp := serve(t, http.MethodPost, "/controller-dated-samples", handler, "/controller-dated-samples", `{"name":"`+name+`","clock":`+value+`}`)
		require.Equal(t, http.StatusBadRequest, rsp.Code, form)
		require.Contains(t, rsp.Body.String(), `"msg":"invalid value for field 'clock'"`, form)
		require.Zero(t, countDatedNamed(t, name), form)
	}
}

// TestCreateAnswersConflictForATakenID pins the 409 of a create whose id a
// stored record already carries, and that the stored record stays as it was.
func TestCreateAnswersConflictForATakenID(t *testing.T) {
	record := createSample(t, "create-taken")

	rsp := serve(t, http.MethodPost, "/controller-samples",
		controller.CreateHandler[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](sampleRoute)),
		"/controller-samples", `{"id":"`+record.GetID()+`","name":"create-other"}`)

	require.Equal(t, http.StatusConflict, rsp.Code)
	requireSampleName(t, record.GetID(), "create-taken")
}
