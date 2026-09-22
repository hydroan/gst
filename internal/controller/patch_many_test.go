package controller_test

import (
	"net/http"
	"testing"

	"github.com/hydroan/gst/internal/controller"
	"github.com/stretchr/testify/require"
)

// TestPatchManyReportsAMissingVersionBeforeAMissingRecord pins which failure
// a batch patch of a versioned model reports for an item that carries no
// version and names a record that does not exist: the missing version, 400,
// because every item's version is checked before any record is loaded.
// Loading the record first would answer 404 instead.
func TestPatchManyReportsAMissingVersionBeforeAMissingRecord(t *testing.T) {
	rsp := serve(t, http.MethodPatch, "/controller-versioned-samples/batch",
		controller.PatchManyFactory[*versionedSample, *versionedSample, *versionedSample](),
		"/controller-versioned-samples/batch", `{"items":[{"id":"missing","name":"renamed"}]}`)

	require.Equal(t, http.StatusBadRequest, rsp.Code)
	require.Contains(t, rsp.Body.String(), `"code":1000`)
}
