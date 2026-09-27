package record_test

import (
	"testing"

	"demo/internal/testsupport"
	"demo/model"

	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
)

// TestCreate covers POST /api/records, served by Creator in create.go: the
// owner is the session's user, the type defaults to text, and the creation
// leaves an audit row.
func TestCreate(t *testing.T) {
	account := testsupport.Login(t)

	record, err := account.Client.Post[model.Record](t.Context(), "/api/records", &model.Record{Title: "first"})
	require.NoError(t, err)
	require.Equal(t, account.UserID, record.UserID)
	require.Equal(t, model.RecordTypeText, record.Type)

	stored := testutil.RequireGet[model.Record](t, record.ID)
	require.Equal(t, "first", stored.Title)
	require.Equal(t, 1, testutil.RequireCount[model.Audit](t, &model.Audit{RecordID: record.ID, Action: "create", Actor: account.Username}))
}
