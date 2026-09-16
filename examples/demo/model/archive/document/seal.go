package document

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Seal demonstrates a custom action model for a document.
type Seal struct {
	model.Empty
}

// SealReq is the request for sealing a document payload.
type SealReq struct {
	DocumentID string `json:"document_id"`
	Content    string `json:"content"`
}

// SealRsp is the response returned after sealing a document.
type SealRsp struct {
	DocumentID string `json:"document_id"`
	Sealed     string `json:"sealed"`
	Algorithm  string `json:"algorithm"`
}

func (Seal) Design() {
	Route("/archive/documents/seal", func() {
		Create(func() {
			Service()
			Payload[*SealReq]()
			Result[*SealRsp]()
		})
	})
}
