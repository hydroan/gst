package document_test

import (
	"testing"

	"demo/internal/testsupport"
	"demo/model/archive"

	"github.com/stretchr/testify/require"
)

// createDocument creates a document in the box and returns it as the API
// answered.
func createDocument(t *testing.T, account *testsupport.Account, boxID, name, content string) *archive.Document {
	t.Helper()
	document, err := account.Client.Post[archive.Document](t.Context(), "/api/archive/documents", &archive.Document{BoxID: boxID, Name: name, Content: content})
	require.NoError(t, err)
	return document
}
