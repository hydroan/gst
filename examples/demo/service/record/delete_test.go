package record_test

import (
	"net/http"
	"testing"

	"demo/internal/testsupport"
	"demo/model"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
)

// TestDelete covers DELETE /api/records/:record, served by the framework
// with the hook of Deleter in delete.go: the caller's record goes, and
// someone else's is not there to delete.
func TestDelete(t *testing.T) {
	account := testsupport.Login(t)
	record := createRecord(t, account, "mine", model.RecordTypeText)

	_, err := testsupport.Login(t).Client.Delete[struct{}](t.Context(), "/api/records/"+record.ID, nil)
	testutil.RequireError(t, err, http.StatusNotFound)
	list, err := account.Client.Get[client.ListResult[*model.Record]](t.Context(), "/api/records")
	require.NoError(t, err)
	require.Equal(t, 1, list.Total, "a delete refused leaves the record")

	_, err = account.Client.Delete[struct{}](t.Context(), "/api/records/"+record.ID, nil)
	require.NoError(t, err)
	list, err = account.Client.Get[client.ListResult[*model.Record]](t.Context(), "/api/records")
	require.NoError(t, err)
	require.Equal(t, 0, list.Total)
}
