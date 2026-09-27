package record_test

import (
	"testing"

	"demo/internal/testsupport"
	"demo/model"

	"github.com/stretchr/testify/require"
)

// TestSummary covers GET /api/records/summary, served by Summary in
// summary.go: the caller's records counted in all and by type.
func TestSummary(t *testing.T) {
	account := testsupport.Login(t)
	createRecord(t, account, "text one", model.RecordTypeText)
	createRecord(t, account, "text two", model.RecordTypeText)
	createRecord(t, account, "image one", model.RecordTypeImage)
	createRecord(t, testsupport.Login(t), "someone else's", model.RecordTypeText)

	summary, err := account.Client.Get[model.RecordSummaryRsp](t.Context(), "/api/records/summary")
	require.NoError(t, err)
	require.Equal(t, 3, summary.Total)
	require.Equal(t, map[model.RecordType]int{model.RecordTypeText: 2, model.RecordTypeImage: 1}, summary.ByType)
}
