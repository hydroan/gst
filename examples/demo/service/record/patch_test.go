package record_test

import (
	"net/http"
	"testing"

	"demo/internal/testsupport"
	"demo/model"

	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
)

// TestPatch covers PATCH /api/records/:record, served by the framework with
// the hook of Patcher in patch.go: the fields the body names change and
// nothing else, the owner stays even when the body names it, the version
// the body carries decides whether the write goes through, and someone
// else's record is not there to patch.
func TestPatch(t *testing.T) {
	account := testsupport.Login(t)
	record := createRecord(t, account, "draft", model.RecordTypeImage)

	patched, err := account.Client.Patch[model.Record](t.Context(), "/api/records/"+record.ID, map[string]any{"title": "final", "version": record.Version})
	require.NoError(t, err)
	require.Equal(t, "final", patched.Title)
	require.Equal(t, model.RecordTypeImage, patched.Type, "a field the body does not name keeps its value")
	require.EqualValues(t, 2, patched.Version)

	other := testsupport.Login(t)
	patched, err = account.Client.Patch[model.Record](t.Context(), "/api/records/"+record.ID, map[string]any{"user_id": other.UserID, "version": patched.Version})
	require.NoError(t, err)
	require.Equal(t, account.UserID, patched.UserID, "the owner stays whatever the body names")

	_, err = account.Client.Patch[model.Record](t.Context(), "/api/records/"+record.ID, map[string]any{"title": "unversioned"})
	testutil.RequireError(t, err, http.StatusBadRequest)

	_, err = other.Client.Patch[model.Record](t.Context(), "/api/records/"+record.ID, map[string]any{"title": "taken", "version": patched.Version})
	testutil.RequireError(t, err, http.StatusNotFound)
}
