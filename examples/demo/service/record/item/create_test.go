package item_test

import (
	"testing"

	"demo/internal/testsupport"
	"demo/model"
	"demo/model/record"

	"github.com/stretchr/testify/require"
)

// TestCreate covers POST /api/records/:record/items, served by Creator in
// create.go: the item belongs to the record of the route.
func TestCreate(t *testing.T) {
	account := testsupport.Login(t)
	parent, err := account.Client.Post[model.Record](t.Context(), "/api/records", &model.Record{Title: "parent"})
	require.NoError(t, err)

	item, err := account.Client.Post[record.Item](t.Context(), "/api/records/"+parent.ID+"/items", &record.Item{Kind: record.ItemKindInput, Content: "one"})
	require.NoError(t, err)
	require.Equal(t, parent.ID, item.RecordID)
	require.NotEmpty(t, item.ID)
}
