package controller_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/controller"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
)

// TestExportAnswersAClientThatWentAwayAsCanceled pins that an export the
// client cut short answers as canceled, the client's doing, not as a failure
// of the server.
func TestExportAnswersAClientThatWentAwayAsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	rsp := serveAs(ctx, t, http.MethodGet, "/controller-samples/export",
		controller.ExportHandler[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](sampleRoute)),
		"/controller-samples/export", "", "alice", nil)

	require.Equal(t, types.StatusClientClosedRequest, rsp.Code, rsp.Body.String())
}

// TestExportRefusesThePagingParameters pins that an export, which lists every
// record the query matches, refuses the paging and cursor parameters a List
// of the same model takes rather than ignoring them.
func TestExportRefusesThePagingParameters(t *testing.T) {
	for _, key := range []string{types.QueryPage, types.QuerySize, types.QueryCursorValue, types.QueryCursorField, types.QueryCursorNext} {
		rsp := serveAs(context.Background(), t, http.MethodGet, "/controller-exports/export",
			controller.ExportHandler[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](exportRoute)),
			"/controller-exports/export?"+key+"=1", "", "alice", nil)
		require.Equal(t, http.StatusBadRequest, rsp.Code, rsp.Body.String())
		require.Contains(t, rsp.Body.String(), `"msg":"unsupported query parameter \"`+key+`\""`)
	}
}

// TestExportTakesALimit pins _limit, the export action's own parameter: it
// caps the records exported, zero or less exporting every one, and a value
// that is no integer is refused rather than read as no cap at all.
func TestExportTakesALimit(t *testing.T) {
	prefix := uniqueName("export-limit")
	for _, suffix := range []string{"-a", "-b", "-c"} {
		createSample(t, prefix+suffix)
	}
	export := func(query string) *httptest.ResponseRecorder {
		return serveAs(context.Background(), t, http.MethodGet, "/controller-exports/export",
			controller.ExportHandler[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](exportRoute)),
			"/controller-exports/export?"+query, "", "alice", nil)
	}

	rsp := export("name[startswith]=" + prefix + "&_format=csv&_limit=2")
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	require.Equal(t, 2, strings.Count(rsp.Body.String(), prefix), rsp.Body.String())

	for _, limit := range []string{"0", "-1"} {
		rsp = export("name[startswith]=" + prefix + "&_limit=" + limit)
		require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
		require.Equal(t, 3, strings.Count(rsp.Body.String(), prefix), "a cap of %s exports every record the query matches", limit)
	}

	rsp = export("name[startswith]=" + prefix + "&_limit=abc")
	require.Equal(t, http.StatusBadRequest, rsp.Code, rsp.Body.String())
	require.Contains(t, rsp.Body.String(), `"msg":"invalid query parameter \"_limit\": expect a numeric value, got \"abc\""`)
}
