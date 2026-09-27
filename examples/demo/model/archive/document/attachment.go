package document

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Attachment is the one file kept beside a document, in object storage: an
// action model on a route under the document. Get is declared Exact so the
// route stays /api/archive/documents/:document/attachment instead of gaining
// an id of its own. The service in service/archive/document/attachment
// reaches MinIO through the framework's provider.
type Attachment struct {
	model.Empty
}

// AttachmentReq is the file to keep.
type AttachmentReq struct {
	Content string `json:"content"`
}

// AttachmentRsp describes the file kept, content included when read back.
type AttachmentRsp struct {
	Key     string `json:"key"`
	Size    int64  `json:"size"`
	Content string `json:"content,omitempty"`
}

func (Attachment) Design() {
	Route("archive/documents/:document/attachment", func() {
		Create(func() {
			Service()
			Payload[*AttachmentReq]()
			Result[*AttachmentRsp]()
		})
		Get(func() {
			Exact()
			Service()
			Result[*AttachmentRsp]()
		})
	})
}
