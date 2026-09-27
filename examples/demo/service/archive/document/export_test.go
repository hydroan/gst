package document_test

import (
	"testing"

	"demo/internal/testsupport"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
)

// TestExport covers GET /api/archive/documents/export, served by Exporter in
// export.go: the documents the query matches, as CSV.
func TestExport(t *testing.T) {
	account := testsupport.Login(t)
	createDocument(t, account, "export-box", "a.txt", "alpha")
	createDocument(t, account, "export-box", "b.txt", "beta")
	createDocument(t, account, "another-box", "c.txt", "gamma")

	rows := testutil.DownloadCSV(t, account.Client, "/api/archive/documents/export", client.WithQuery("box_id", "export-box"))
	require.Equal(t, [][]string{
		{"box_id", "name", "format", "content"},
		{"export-box", "a.txt", "text", "alpha"},
		{"export-box", "b.txt", "text", "beta"},
	}, rows)
}
