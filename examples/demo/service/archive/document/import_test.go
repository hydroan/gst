package document_test

import (
	"net/http"
	"strings"
	"testing"

	"demo/internal/testsupport"
	"demo/model/archive"

	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
)

// TestImport covers POST /api/archive/documents/import, served by Importer
// in import.go: the rows of the uploaded CSV become documents, checked and
// completed by the model hooks like any other creation, and a file whose
// header lacks the four columns is refused.
func TestImport(t *testing.T) {
	account := testsupport.Login(t)

	file := strings.NewReader("box_id,name,format,content\nbox-1,notes.md,markdown,# notes\nbox-1,todo.txt,,buy milk\n")
	_, err := account.Client.Upload(t.Context(), "/api/archive/documents/import", "documents.csv", file, nil)
	require.NoError(t, err)

	documents := testutil.RequireList[archive.Document](t, &archive.Document{BoxID: "box-1"}, archive.DocumentCols.Name.Asc())
	require.Len(t, documents, 2)
	require.Equal(t, archive.DocumentFormatMarkdown, documents[0].Format)
	require.Equal(t, archive.DocumentFormatText, documents[1].Format, "the hook defaulted the format")
	require.Equal(t, len("buy milk"), documents[1].Size)

	narrow := strings.NewReader("box_id,name\nbox-1,notes.md\n")
	_, err = account.Client.Upload(t.Context(), "/api/archive/documents/import", "documents.csv", narrow, nil)
	testutil.RequireError(t, err, http.StatusBadRequest, "the header needs the columns box_id, name, format and content")
}
