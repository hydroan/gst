package record_test

import (
	"net/http"
	"testing"

	"demo/internal/testsupport"
	"demo/model"

	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
)

// TestUpdate covers PUT /api/records/:record, served by the framework with
// the hook of Updater in update.go: the body replaces the row and the owner
// stays, and the version the body carries decides whether the write goes
// through, a stale one answering 409 and none 400; someone else's record is
// not there to update.
func TestUpdate(t *testing.T) {
	account := testsupport.Login(t)
	record := createRecord(t, account, "draft", model.RecordTypeText)
	require.EqualValues(t, 1, record.Version)

	updated, err := account.Client.Put[model.Record](t.Context(), "/api/records/"+record.ID, &model.Record{Title: "final", Type: model.RecordTypeText, Version: record.Version})
	require.NoError(t, err)
	require.Equal(t, "final", updated.Title)
	require.Equal(t, account.UserID, updated.UserID, "the owner stays whatever the body names")
	require.EqualValues(t, 2, updated.Version)

	_, err = account.Client.Put[model.Record](t.Context(), "/api/records/"+record.ID, &model.Record{Title: "stale", Type: model.RecordTypeText, Version: record.Version})
	testutil.RequireError(t, err, http.StatusConflict)

	_, err = account.Client.Put[model.Record](t.Context(), "/api/records/"+record.ID, &model.Record{Title: "unversioned", Type: model.RecordTypeText})
	testutil.RequireError(t, err, http.StatusBadRequest)

	_, err = testsupport.Login(t).Client.Put[model.Record](t.Context(), "/api/records/"+record.ID, &model.Record{Title: "taken", Type: model.RecordTypeText, Version: updated.Version})
	testutil.RequireError(t, err, http.StatusNotFound)
}
