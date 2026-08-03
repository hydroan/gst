package client

import (
	"bytes"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httputil"
	"strings"

	"github.com/cockroachdb/errors"
)

// Do sends one request and parses the response envelope. It is the non-generic
// floor under the verb functions: use it when the caller needs envelope
// details such as TraceID or Cookies instead of a decoded payload.
func (c *Client) Do(method, path string, payload any, opts ...RequestOption) (*Resp, error) {
	req, err := c.newRequest(method, path, payload, opts)
	if err != nil {
		return nil, err
	}
	return c.roundTrip(req)
}

// newRequest builds the HTTP request: URL from the service address plus path
// and encoded query parameters, body from payload, headers and credentials
// from the client.
func (c *Client) newRequest(method, path string, payload any, opts []RequestOption) (*http.Request, error) {
	if !strings.HasPrefix(c.addr, "http://") && !strings.HasPrefix(c.addr, "https://") {
		return nil, errors.New("addr must start with http:// or https://")
	}

	encoded, err := newRequestConfig(opts).encode()
	if err != nil {
		return nil, errors.Wrap(err, "failed to encode the query parameters")
	}
	url := c.addr
	if path != "" {
		url = c.addr + "/" + strings.TrimLeft(path, "/")
	}
	if encoded != "" {
		url += "?" + encoded
	}

	var reader io.Reader
	if payload != nil {
		switch v := payload.(type) {
		case []byte:
			reader = bytes.NewReader(v)
		default:
			data, marshalErr := json.Marshal(v)
			if marshalErr != nil {
				return nil, errors.Wrap(marshalErr, "failed to marshal the payload")
			}
			reader = bytes.NewReader(data)
		}
	}

	req, err := http.NewRequestWithContext(c.ctx, method, url, reader)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create the request")
	}
	if len(c.username) > 0 {
		req.SetBasicAuth(c.username, c.password)
	}
	if len(c.token) > 0 {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	maps.Copy(req.Header, c.header)
	return req, nil
}

// roundTrip sends the request and parses the envelope.
func (c *Client) roundTrip(req *http.Request) (*Resp, error) {
	if c.debug {
		if dump, err := httputil.DumpRequest(req, true); err == nil {
			c.Infoz(string(dump))
		}
	}

	httpRsp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to send the request")
	}
	defer httpRsp.Body.Close()

	body, err := io.ReadAll(httpRsp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read the response body")
	}
	if c.debug {
		if dump, err := httputil.DumpResponse(httpRsp, false); err == nil {
			c.Infoz(string(dump) + string(body))
		}
	}
	return parseEnvelope(httpRsp, body)
}

// parseEnvelope turns one HTTP response into a Resp, or an *Error when the
// server rejected the request: a non-2xx status, or a non-zero envelope code.
// The envelope decode is best-effort so a non-JSON error page still produces
// an *Error carrying the raw body.
func parseEnvelope(httpRsp *http.Response, body []byte) (*Resp, error) {
	res := new(Resp)
	if len(body) > 0 {
		_ = json.Unmarshal(body, res)
	}

	ok := httpRsp.StatusCode >= 200 && httpRsp.StatusCode < 300
	if !ok || res.Code != 0 {
		msg := res.Msg
		if msg == "" {
			// Middleware rejections write {"error": "..."} instead of the
			// standard envelope; surface that text as the message.
			var legacy struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(body, &legacy); err == nil {
				msg = legacy.Error
			}
		}
		return nil, &Error{
			StatusCode: httpRsp.StatusCode,
			Code:       res.Code,
			Msg:        msg,
			TraceID:    res.TraceID,
			Body:       body,
		}
	}

	res.Cookies = httpRsp.Cookies()
	return res, nil
}
