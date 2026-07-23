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

// WrappedResponse wraps *http.Response so that it survives a JSON
// marshal/unmarshal round trip, including the response body and TLS state.
type WrappedResponse struct {
	*http.Response
}

// NOTE: do not add "omitempty" to the json tags, otherwise the jsoniter
// package fails to marshal/unmarshal this type. sonic and encoding/json
// handle WrappedResponse fine, while go-json fails with
// "unsupported type: func() io.ReadCloser".
type wrappedResponse struct {
	Request string          `json:"Request"`
	Body    json.RawMessage `json:"Body"`
	TLS     json.RawMessage `json:"TLS"`
	*http.Response
}

// wrappedTLS mirrors tls.ConnectionState with the certificate chains
// replaced by raw DER bytes, which encoding/json encodes as base64 strings.
type wrappedTLS struct {
	PeerCertificates [][]byte   `json:"PeerCertificates"`
	VerifiedChains   [][][]byte `json:"VerifiedChains"`
	*tls.ConnectionState
}

// MarshalJSON implements json.Marshaler. It drains Response.Body and
// restores it afterwards so that the response stays usable. The body must
// be valid JSON: it is embedded verbatim to keep the marshaled form
// readable. An empty body and an absent TLS state are encoded as null
// because an empty json.RawMessage cannot be marshaled.
func (r *WrappedResponse) MarshalJSON() ([]byte, error) {
	var err error
	var body []byte
	var tlsState []byte

	if r.Response.Body != nil {
		body, err = io.ReadAll(r.Response.Body)
		if err != nil {
			return nil, err
		}
		r.Response.Body = io.NopCloser(bytes.NewReader(body))
	}
	if len(body) == 0 {
		body = nil
	}

	if r.Response.TLS != nil {
		rtls := new(wrappedTLS)
		rtls.ConnectionState = r.Response.TLS
		for _, cert := range r.Response.TLS.PeerCertificates {
			rtls.PeerCertificates = append(rtls.PeerCertificates, cert.Raw)
		}
		for i := range r.Response.TLS.VerifiedChains {
			var chain [][]byte
			for _, cert := range r.Response.TLS.VerifiedChains[i] {
				chain = append(chain, cert.Raw)
			}
			rtls.VerifiedChains = append(rtls.VerifiedChains, chain)
		}
		if tlsState, err = json.Marshal(rtls); err != nil {
			return nil, err
		}
	}

	return json.Marshal(&wrappedResponse{
		Body:     body,
		TLS:      tlsState,
		Response: r.Response,
	})
}

// UnmarshalJSON implements json.Unmarshaler. The restored Response.Body is
// always a non-nil reader; a body marshaled as null yields an empty one.
func (r *WrappedResponse) UnmarshalJSON(data []byte) error {
	wresp := new(wrappedResponse)
	if err := json.Unmarshal(data, wresp); err != nil {
		return err
	}
	var body []byte
	if !isJSONNull(wresp.Body) {
		body = wresp.Body
	}
	r.Response = wresp.Response
	r.Response.Body = io.NopCloser(bytes.NewReader(body))

	if !isJSONNull(wresp.TLS) {
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
// func (r *WrappedResponse) MarshalBinary() ([]byte, error) {
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
// }
//
//// UnmarshalBinary
// func (r *WrappedResponse) UnmarshalBinary(data []byte) error {
//    wresp := new(wrappedResponse)
//    if err := binary.Unmarshal(data, wresp); err != nil {
//        return err
//    }
//    r.Response = wresp.Response
//    r.Response.Body = io.NopCloser(bytes.NewReader(wresp.Body))
//    return nil
// }
