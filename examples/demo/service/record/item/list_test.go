package item_test

import (
	"testing"

	"demo/internal/testsupport"
	"demo/model"
	"demo/model/record"

	"github.com/hydroan/gst/client"
	"github.com/stretchr/testify/require"
)

// TestList covers GET /api/records/:record/items, served by Lister in
// list.go: the items of the record of the route, and no other's.
func TestList(t *testing.T) {
	account := testsupport.Login(t)
	parents := make([]*model.Record, 0, 2)
	for _, title := range []string{"one", "two"} {
		parent, err := account.Client.Post[model.Record](t.Context(), "/api/records", &model.Record{Title: title})
		require.NoError(t, err)
		parents = append(parents, parent)
		for _, content := range []string{title + " a", title + " b"} {
			_, err := account.Client.Post[record.Item](t.Context(), "/api/records/"+parent.ID+"/items", &record.Item{Kind: record.ItemKindInput, Content: content})
			require.NoError(t, err)
		}
	}

	list, err := account.Client.Get[client.ListResult[record.Item]](t.Context(), "/api/records/"+parents[0].ID+"/items")
	require.NoError(t, err)
	require.Equal(t, 2, list.Total)
	for _, item := range list.Items {
		require.Equal(t, parents[0].ID, item.RecordID)
	}
}
