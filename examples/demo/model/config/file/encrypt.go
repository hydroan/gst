package file

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Encrypt demonstrates a custom action model for a configuration file.
type Encrypt struct {
	model.Empty
}

// EncryptReq is the request for encrypting a configuration file payload.
type EncryptReq struct {
	FileID    string `json:"file_id"`
	Plaintext string `json:"plaintext"`
}

// EncryptRsp is the response returned after encrypting a configuration file.
type EncryptRsp struct {
	FileID     string `json:"file_id"`
	Ciphertext string `json:"ciphertext"`
	Algorithm  string `json:"algorithm"`
}

func (Encrypt) Design() {
	Route("/config/files/encrypt", func() {
		Create(func() {
			Service(true)
			Payload[*EncryptReq]()
			Result[*EncryptRsp]()
		})
	})
}
