package attachment

import (
	"net/http"
	"strings"

	"demo/model/archive"
	"demo/model/archive/document"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/provider/minio"
	"github.com/hydroan/gst/service"
)

// Creator serves POST /api/archive/documents/:document/attachment.
type Creator struct {
	service.Base[*document.Attachment, *document.AttachmentReq, *document.AttachmentRsp]
}

// Create keeps the content in object storage under the document's key,
// through the framework's MinIO provider, which the [minio] section
// configures and the startup connects. The document is read first, so an
// attachment is never kept for a document that does not exist.
func (a *Creator) Create(ctx *gst.ServiceContext, req *document.AttachmentReq) (*document.AttachmentRsp, error) {
	documentID := ctx.Param("document")
	if err := database.Database[*archive.Document](ctx).Get(new(archive.Document), documentID); err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			return nil, gst.NewErrorWithCause(http.StatusNotFound, "document not found", err)
		}
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "failed to load the document", err)
	}
	info, err := minio.Put(ctx, attachmentKey(documentID), strings.NewReader(req.Content), &minio.PutOptions{
		ContentType: "text/plain",
		Size:        int64(len(req.Content)),
	})
	if err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "failed to store the attachment", err)
	}
	return &document.AttachmentRsp{Key: info.Key, Size: info.Size}, nil
}
