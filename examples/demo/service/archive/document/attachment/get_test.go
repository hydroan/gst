package attachment_test

import (
	"testing"

	"demo/internal/testsupport"
	"demo/model/archive/document"

	"github.com/stretchr/testify/require"
)

// TestGet covers GET /api/archive/documents/:document/attachment, served by
// Getter in get.go: the attachment read back from object storage.
func TestGet(t *testing.T) {
	account := testsupport.Login(t)
	doc := createDocument(t, account)
	path := "/api/archive/documents/" + doc.ID + "/attachment"

	_, err := account.Client.Post[document.AttachmentRsp](t.Context(), path, &document.AttachmentReq{Content: "attached text"})
	require.NoError(t, err)
	rsp, err := account.Client.Get[document.AttachmentRsp](t.Context(), path)
	require.NoError(t, err)
	require.Equal(t, "attached text", rsp.Content)
	require.Equal(t, int64(len("attached text")), rsp.Size)
}
