package httpwrapper

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

// references: https://avi.im/blag/2021/golang-marshaling-special-fields/

// Body 如果为nil, 可能会有 "invalid character '\\x00' looking for beginning of value" 错误
// 还未完全验证.

type WrappedRequest struct {
	*http.Request
}

// NOTE: json tag 不要加 "omitempty", 否则 jsoniter 包在序列化和反序列化的过程中会报错
// 测试发现, snoic, encoding/json 这两个包可以 marshal/unmarshal WrappedRequest
// go-json 会报错: "unsupported type: func() io.ReadCloser,"
type wrappedRequest struct {
	Body    json.RawMessage `json:"Body"`
	GetBody string          `json:"GetBody"`
	Cancel  string          `json:"Cancel"`
	*http.Request
}

// MarshalJSON
func (r *WrappedRequest) MarshalJSON() ([]byte, error) {
	var err error
	var body []byte

	if r.Request.Body != nil {
		if body, err = io.ReadAll(r.Request.Body); err != nil {
			return nil, err
		}
		r.Request.Body = io.NopCloser(bytes.NewReader(body))
	}

	//nolint:staticcheck
	return json.Marshal(&wrappedRequest{
		Body:    body,
		Request: r.Request,
	})
}

// UnmarshalJSON
func (r *WrappedRequest) UnmarshalJSON(data []byte) error {
	wreq := new(wrappedRequest)
	if err := json.Unmarshal(data, wreq); err != nil {
		return err
	}
	r.Request = wreq.Request
	r.Request.Body = io.NopCloser(bytes.NewReader(wreq.Body))
	return nil
}

//// MarshalBinary
//func (r *WrappedRequest) MarshalBinary() ([]byte, error) {
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
//}
//
//// UnmarshalBinary
//func (r *WrappedRequest) UnmarshalBinary(data []byte) error {
//    wreq := new(wrappedRequest)
//    if err := binary.Unmarshal(data, wreq); err != nil {
//        return err
//    }
//    r.Request = wreq.Request
//    r.Request.Body = io.NopCloser(bytes.NewReader(wreq.Body))
//    return nil
//}
