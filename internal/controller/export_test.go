package controller_test

import (
	"context"
	"net/http"
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
