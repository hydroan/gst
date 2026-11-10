package record_test

import (
	"testing"

	"demo/model"

	"github.com/hydroan/gst/testutil/iamtest"
	"github.com/stretchr/testify/require"
)

// createRecord creates a record of the account's, of the type given, and
// returns it as the API answered.
func createRecord(t *testing.T, account *iamtest.Account, title string, kind model.RecordType) *model.Record {
	t.Helper()
	record, err := account.Client.Post[model.Record](t.Context(), "/api/records", &model.Record{Title: title, Type: kind})
	require.NoError(t, err)
	return record
}
