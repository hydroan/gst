package item_test

import (
	"testing"

	"demo/internal/testsupport"
	"demo/model"
	"demo/model/record"

	"github.com/stretchr/testify/require"
)

// TestSummary covers GET /api/items/summary, served by Summary in
// summary.go: the caller's items counted by the type of their record, and
// nobody else's.
func TestSummary(t *testing.T) {
	account := testsupport.Login(t)
	for kind, contents := range map[model.RecordType][]string{
		model.RecordTypeText:  {"one", "two"},
		model.RecordTypeImage: {"three"},
	} {
		parent, err := account.Client.Post[model.Record](t.Context(), "/api/records", &model.Record{Title: string(kind), Type: kind})
		require.NoError(t, err)
		for _, content := range contents {
			_, err := account.Client.Post[record.Item](t.Context(), "/api/records/"+parent.ID+"/items", &record.Item{Kind: record.ItemKindInput, Content: content})
			require.NoError(t, err)
		}
	}
	other := testsupport.Login(t)
	parent, err := other.Client.Post[model.Record](t.Context(), "/api/records", &model.Record{Title: "someone else's"})
	require.NoError(t, err)
	_, err = other.Client.Post[record.Item](t.Context(), "/api/records/"+parent.ID+"/items", &record.Item{Kind: record.ItemKindOutput, Content: "theirs"})
	require.NoError(t, err)

	summary, err := account.Client.Get[record.ItemSummaryRsp](t.Context(), "/api/items/summary")
	require.NoError(t, err)
	require.Equal(t, map[string]int{"text": 2, "image": 1}, summary.ByRecordType)
}
