package record_test

import (
	"testing"

	"demo/internal/testsupport"
	"demo/model"
	"demo/model/record"

	"github.com/hydroan/gst/client"
	"github.com/stretchr/testify/require"
)

// TestList covers GET /api/records, served by Lister in list.go: a list is
// the caller's own records, each carrying the caller's name, and the
// framework's query parameters narrow it further.
func TestList(t *testing.T) {
	alice := testsupport.Login(t)
	bob := testsupport.Login(t)
	createRecord(t, alice, "alice one", model.RecordTypeText)
	createRecord(t, alice, "alice two", model.RecordTypeImage)
	createRecord(t, bob, "bob one", model.RecordTypeText)

	list, err := alice.Client.Get[client.ListResult[model.Record]](t.Context(), "/api/records")
	require.NoError(t, err)
	require.Equal(t, 2, list.Total)
	require.Len(t, list.Items, 2)
	for _, record := range list.Items {
		require.Equal(t, alice.UserID, record.UserID)
		require.Equal(t, alice.Username, record.Username)
	}

	list, err = bob.Client.Get[client.ListResult[model.Record]](t.Context(), "/api/records")
	require.NoError(t, err)
	require.Equal(t, 1, list.Total)
	require.Equal(t, "bob one", list.Items[0].Title)

	// The framework parses the query parameters of its own list, the
	// model embedding model.Query: a filter narrows it within the owner's.
	list, err = alice.Client.Get[client.ListResult[model.Record]](t.Context(), "/api/records", client.WithQuery("type[in]", "image"))
	require.NoError(t, err)
	require.Equal(t, 1, list.Total)
	require.Equal(t, "alice two", list.Items[0].Title)
}

// TestListExpandsTheItems covers the Items association Record declares
// through Expands, on the list Lister hooks and on a get: asked for
// ?_expand=items, the framework answers each record with its items, and
// not asked, without.
func TestListExpandsTheItems(t *testing.T) {
	account := testsupport.Login(t)
	parent := createRecord(t, account, "with items", model.RecordTypeText)
	for _, content := range []string{"one", "two"} {
		_, err := account.Client.Post[record.Item](t.Context(), "/api/records/"+parent.ID+"/items", &record.Item{Kind: record.ItemKindInput, Content: content})
		require.NoError(t, err)
	}

	bare, err := account.Client.Get[client.ListResult[model.Record]](t.Context(), "/api/records")
	require.NoError(t, err)
	require.Len(t, bare.Items, 1)
	require.Empty(t, bare.Items[0].Items)

	expanded, err := account.Client.Get[client.ListResult[model.Record]](t.Context(), "/api/records", client.WithExpand("items", 1))
	require.NoError(t, err)
	require.Len(t, expanded.Items, 1)
	require.Len(t, expanded.Items[0].Items, 2)

	got, err := account.Client.Get[model.Record](t.Context(), "/api/records/"+parent.ID, client.WithExpand("items", 1))
	require.NoError(t, err)
	require.Len(t, got.Items, 2)
	for _, item := range got.Items {
		require.Equal(t, parent.ID, item.RecordID)
	}
}
