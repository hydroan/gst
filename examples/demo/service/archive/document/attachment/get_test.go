package attachment_test

import (
	"net/http"
	"testing"

	"demo/internal/testsupport"
	"demo/model/archive/document"

	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
)

// TestGet covers GET /api/archive/documents/:document/attachment, served by
// Getter in get.go: a document without an attachment answers 404, and the
// attachment, once posted, is read back from object storage.
func TestGet(t *testing.T) {
	account := testsupport.Login(t)
	doc := createDocument(t, account)
	path := "/api/archive/documents/" + doc.ID + "/attachment"

	_, err := account.Client.Get[document.AttachmentRsp](t.Context(), path)
	testutil.RequireError(t, err, http.StatusNotFound, "attachment not found")

	_, err = account.Client.Post[document.AttachmentRsp](t.Context(), path, &document.AttachmentReq{Content: "attached text"})
	require.NoError(t, err)
	rsp, err := account.Client.Get[document.AttachmentRsp](t.Context(), path)
	require.NoError(t, err)
	require.Equal(t, "attached text", rsp.Content)
	require.Equal(t, int64(len("attached text")), rsp.Size)
}
