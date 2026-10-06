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

// Get reads the attachment back from object storage: a document without one
// answers 404, a store that cannot be read 500.
func (a *Getter) Get(ctx *gst.ServiceContext, _ *model.Empty) (*document.AttachmentRsp, error) {
	key := attachmentKey(ctx.Param("document"))
	found, err := minio.Exists(ctx, key)
	if err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "failed to look the attachment up", err)
	}
	if !found {
		return nil, gst.NewError(http.StatusNotFound, "attachment not found")
	}
	object, info, err := minio.Get(ctx, key)
	if err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "failed to read the attachment", err)
	}
	defer object.Close()
	content, err := io.ReadAll(object)
	if err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "failed to read the attachment", err)
	}
	return &document.AttachmentRsp{Key: info.Key, Size: info.Size, Content: string(content)}, nil
}
