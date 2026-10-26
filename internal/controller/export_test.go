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

// TestExportTakesALimit pins _limit, the export action's own parameter: it
// caps the records exported, and a value that is no integer is refused rather
// than read as no cap at all.
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

	rsp = export("name[startswith]=" + prefix + "&_limit=abc")
	require.Equal(t, http.StatusBadRequest, rsp.Code, rsp.Body.String())
	require.Contains(t, rsp.Body.String(), `"msg":"invalid query parameter \"_limit\": expect a numeric value, got \"abc\""`)
}
