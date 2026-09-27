package record_test

import (
	"testing"

	"demo/internal/testsupport"
	"demo/model"

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
