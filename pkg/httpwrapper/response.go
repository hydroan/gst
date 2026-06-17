package httpwrapper

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
)

// references: https://avi.im/blag/2021/golang-marshaling-special-fields/

type WrappedResponse struct {
	*http.Response
}

// NOTE: json tag 不要加 "omitempty", 否则 jsoniter 包在序列化和反序列化的过程中会报错
// 测试发现, snoic, encoding/json 这两个包可以 marshal/unmarshal WrappedRequest
// go-json 会报错: "unsupported type: func() io.ReadCloser,"
type wrappedResponse struct {
	Request string          `json:"Request"`
	Body    json.RawMessage `json:"Body"`
	TLS     json.RawMessage `json:"TLS"`
	*http.Response
}
type wrappedTLS struct {
	PeerCertificates []json.RawMessage   `json:"PeerCertificates"`
	VerifiedChains   [][]json.RawMessage `json:"VerifiedChains"`
	*tls.ConnectionState
}

// MarshalJSON
//
// wrappedResponse.Body 如果是 []byte 类型, 默认值一定要是 []byte{}, 而不是 nil.
// 当 Body 的类型为 string 时, 空字符串 json.Marshal 后的结果就是 "".
// 所以这个 body 一定要用 []byte{}, 这样 json.Marshal 出来之后就是 "".
// nil 在 json.Marshal 后的结果是 null, 这样就与 string 类型的 Body 不匹配.
func (r *WrappedResponse) MarshalJSON() ([]byte, error) {
	var err error
	var body []byte
	var tls []byte

	// 1.check Response.Body
	if r.Response.Body != nil {
		body, err = io.ReadAll(r.Response.Body)
		if err != nil {
			return nil, err
		}
		r.Response.Body = io.NopCloser(bytes.NewReader(body))
	}

	// 2.check Response.TLS
	if r.Response.TLS != nil {
		rtls := new(wrappedTLS)
		rtls.ConnectionState = r.Response.TLS
		for _, cert := range r.Response.TLS.PeerCertificates {
			rtls.PeerCertificates = append(rtls.PeerCertificates, cert.Raw)
		}
		for i := range r.Response.TLS.VerifiedChains {
			var chain []json.RawMessage
			for _, cert := range r.Response.TLS.VerifiedChains[i] {
				chain = append(chain, cert.Raw)
			}
			rtls.VerifiedChains = append(rtls.VerifiedChains, chain)
		}
		if tls, err = json.Marshal(rtls); err != nil {
			return nil, err
		}
	}

	return json.Marshal(&wrappedResponse{
		Body:     body,
		TLS:      tls,
		Response: r.Response,
	})
}

// UnmarshalJSON
func (r *WrappedResponse) UnmarshalJSON(data []byte) error {
	wresp := new(wrappedResponse)
	if err := json.Unmarshal(data, wresp); err != nil {
		return err
	}
	r.Response = wresp.Response
	r.Response.Body = io.NopCloser(bytes.NewReader(wresp.Body))

	if len(wresp.TLS) != 0 {
		rtls := new(wrappedTLS)
		if err := json.Unmarshal(wresp.TLS, rtls); err != nil {
			return err
		}
		var peerCertificates []*x509.Certificate
		for _, der := range rtls.PeerCertificates {
			cert, err := x509.ParseCertificate(der)
			if err != nil {
				return err
			}
			peerCertificates = append(peerCertificates, cert)
		}
		var verifiedChains [][]*x509.Certificate
		for i := range rtls.VerifiedChains {
			var chains []*x509.Certificate
			for _, der := range rtls.VerifiedChains[i] {
				cert, err := x509.ParseCertificate(der)
				if err != nil {
					return err
				}
				chains = append(chains, cert)
			}
			verifiedChains = append(verifiedChains, chains)
		}
		r.Response.TLS = rtls.ConnectionState
		r.Response.TLS.PeerCertificates = peerCertificates
		r.Response.TLS.VerifiedChains = verifiedChains
	}

	return nil
}

//// MarshalBinary
//func (r *WrappedResponse) MarshalBinary() ([]byte, error) {
//    if r.Response.Body == nil {
//        return binary.Marshal(&wrappedResponse{
//            Body:     nil,
//            Response: r.Response,
//        })
//    }
//    body, err := io.ReadAll(r.Response.Body)
//    if err != nil {
//        return nil, err
//    }
//    r.Response.Body = io.NopCloser(bytes.NewReader(body))
//
//    return binary.Marshal(&wrappedResponse{
//        Body:     body,
//        Response: r.Response,
//    })
//}
//
//// UnmarshalBinary
//func (r *WrappedResponse) UnmarshalBinary(data []byte) error {
//    wresp := new(wrappedResponse)
//    if err := binary.Unmarshal(data, wresp); err != nil {
//        return err
//    }
//    r.Response = wresp.Response
//    r.Response.Body = io.NopCloser(bytes.NewReader(wresp.Body))
//    return nil
//}
