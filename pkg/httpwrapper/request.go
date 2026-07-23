package httpwrapper

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

// references: https://avi.im/blag/2021/golang-marshaling-special-fields/

// WrappedRequest wraps *http.Request so that it survives a JSON
// marshal/unmarshal round trip, including the request body.
type WrappedRequest struct {
	*http.Request
}

// NOTE: do not add "omitempty" to the json tags, otherwise the jsoniter
// package fails to marshal/unmarshal this type. sonic and encoding/json
// handle WrappedRequest fine, while go-json fails with
// "unsupported type: func() io.ReadCloser".
type wrappedRequest struct {
	Body    json.RawMessage `json:"Body"`
	GetBody string          `json:"GetBody"`
	Cancel  string          `json:"Cancel"`
	*http.Request
}

// MarshalJSON implements json.Marshaler. It drains Request.Body and restores
// it afterwards so that the request stays usable. The body must be valid
// JSON: it is embedded verbatim to keep the marshaled form readable. An
// empty body is encoded as null because an empty json.RawMessage cannot be
// marshaled.
func (r *WrappedRequest) MarshalJSON() ([]byte, error) {
	var err error
	var body []byte

	if r.Request.Body != nil {
		if body, err = io.ReadAll(r.Request.Body); err != nil {
			return nil, err
		}
		r.Request.Body = io.NopCloser(bytes.NewReader(body))
	}
	if len(body) == 0 {
		body = nil
	}

	//nolint:staticcheck
	return json.Marshal(&wrappedRequest{
		Body:    body,
		Request: r.Request,
	})
}

// UnmarshalJSON implements json.Unmarshaler. The restored Request.Body is
// always a non-nil reader; a body marshaled as null yields an empty one.
func (r *WrappedRequest) UnmarshalJSON(data []byte) error {
	wreq := new(wrappedRequest)
	if err := json.Unmarshal(data, wreq); err != nil {
		return err
	}
	var body []byte
	if !isJSONNull(wreq.Body) {
		body = wreq.Body
	}
	r.Request = wreq.Request
	r.Request.Body = io.NopCloser(bytes.NewReader(body))
	return nil
}

//// MarshalBinary
// func (r *WrappedRequest) MarshalBinary() ([]byte, error) {
//    if r.Request.Body == nil {
//        return binary.Marshal(&wrappedRequest{
//            Body:    nil,
//            Request: r.Request,
//        })
//    }
//    body, err := io.ReadAll(r.Request.Body)
//    if err != nil {
//        return nil, err
//    }
//    r.Request.Body = io.NopCloser(bytes.NewReader(body))
//
//    return binary.Marshal(&wrappedRequest{
//        Body:    body,
//        Request: r.Request,
//    })
// }
//
//// UnmarshalBinary
// func (r *WrappedRequest) UnmarshalBinary(data []byte) error {
//    wreq := new(wrappedRequest)
//    if err := binary.Unmarshal(data, wreq); err != nil {
//        return err
//    }
//    r.Request = wreq.Request
//    r.Request.Body = io.NopCloser(bytes.NewReader(wreq.Body))
//    return nil
// }
