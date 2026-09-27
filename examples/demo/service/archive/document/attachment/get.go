package attachment

import (
	"io"
	"net/http"

	"demo/model/archive/document"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/provider/minio"
	"github.com/hydroan/gst/service"
)

// Getter serves GET /api/archive/documents/:document/attachment, the route
// as declared: the action is Exact, so no id of its own is appended.
type Getter struct {
	service.Base[*document.Attachment, *model.Empty, *document.AttachmentRsp]
}

// Get reads the attachment back from object storage.
func (a *Getter) Get(ctx *gst.ServiceContext, _ *model.Empty) (*document.AttachmentRsp, error) {
	object, info, err := minio.Get(ctx, attachmentKey(ctx.Param("document")))
	if err != nil {
		return nil, service.NewErrorWithCause(http.StatusNotFound, "attachment not found", err)
	}
	defer object.Close()
	content, err := io.ReadAll(object)
	if err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to read the attachment", err)
	}
	return &document.AttachmentRsp{Key: info.Key, Size: info.Size, Content: string(content)}, nil
}
