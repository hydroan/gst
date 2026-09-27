package attachment_test

import (
	"testing"

	"demo/internal/testsupport"
	"demo/model/archive"

	"github.com/stretchr/testify/require"
)

// createDocument creates the document an attachment is kept beside.
func createDocument(t *testing.T, account *testsupport.Account) *archive.Document {
	t.Helper()
	document, err := account.Client.Post[archive.Document](t.Context(), "/api/archive/documents", &archive.Document{BoxID: "box", Name: "with-attachment.txt", Content: "body"})
	require.NoError(t, err)
	return document
}
