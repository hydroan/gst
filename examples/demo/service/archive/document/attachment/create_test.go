package attachment_test

import (
	"testing"

	"demo/internal/testsupport"
	"demo/model/archive/document"

	"github.com/stretchr/testify/require"
)

// TestCreate covers POST /api/archive/documents/:document/attachment,
// served by Creator in create.go: the content lands in object storage under
// the document's key.
func TestCreate(t *testing.T) {
	account := testsupport.Login(t)
	doc := createDocument(t, account)

	rsp, err := account.Client.Post[document.AttachmentRsp](t.Context(), "/api/archive/documents/"+doc.ID+"/attachment", &document.AttachmentReq{Content: "attached text"})
	require.NoError(t, err)
	require.Equal(t, "documents/"+doc.ID+"/attachment", rsp.Key)
	require.Equal(t, int64(len("attached text")), rsp.Size)
}
