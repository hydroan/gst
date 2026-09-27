package document_test

import (
	"testing"

	"demo/internal/testsupport"
	"demo/model/archive"

	"github.com/hydroan/gst/client"
	"github.com/stretchr/testify/require"
)

// TestListByBox covers GET /api/archive/boxes/:box_id/documents, served by
// ListByBox in list_by_box.go: the documents of the box of the route, and
// no other's.
func TestListByBox(t *testing.T) {
	account := testsupport.Login(t)
	createDocument(t, account, "box-a", "a.txt", "alpha")
	createDocument(t, account, "box-a", "b.txt", "beta")
	createDocument(t, account, "box-b", "c.txt", "gamma")

	list, err := account.Client.Get[client.ListResult[archive.Document]](t.Context(), "/api/archive/boxes/box-a/documents")
	require.NoError(t, err)
	require.Equal(t, 2, list.Total)
	for _, document := range list.Items {
		require.Equal(t, "box-a", document.BoxID)
	}
}
