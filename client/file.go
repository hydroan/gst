package client

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/http"

	"github.com/cockroachdb/errors"
)

// Attachment is one downloaded file attachment.
type Attachment struct {
	Name        string // file name parsed from Content-Disposition
	ContentType string
	Content     []byte
}

// Download sends a GET request and reads the response as a file attachment,
// pairing the framework's Export action. A rejection answers with the regular
// JSON envelope and surfaces as an *Error.
func (c *Client) Download(path string, opts ...RequestOption) (*Attachment, error) {
	req, err := c.newRequest(http.MethodGet, path, nil, opts)
	if err != nil {
		return nil, err
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
	if httpRsp.StatusCode < 200 || httpRsp.StatusCode >= 300 {
		_, envErr := parseEnvelope(httpRsp, body)
		return nil, envErr
	}

	attachment := &Attachment{ContentType: httpRsp.Header.Get("Content-Type"), Content: body}
	if disposition := httpRsp.Header.Get("Content-Disposition"); disposition != "" {
		if _, params, err := mime.ParseMediaType(disposition); err == nil {
			attachment.Name = params["filename"]
		}
	}
	return attachment, nil
}

// Upload sends content as the multipart "file" field, pairing the framework's
// Import action. fields are written as plain form fields next to the file.
func (c *Client) Upload(path, filename string, content io.Reader, fields map[string]string) (*Envelope, error) {
	buf := new(bytes.Buffer)
	writer := multipart.NewWriter(buf)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create the multipart file field")
	}
	if _, err = io.Copy(part, content); err != nil {
		return nil, errors.Wrap(err, "failed to write the multipart file content")
	}
	for key, value := range fields {
		if err = writer.WriteField(key, value); err != nil {
			return nil, errors.Wrap(err, "failed to write the multipart field")
		}
	}
	if err = writer.Close(); err != nil {
		return nil, errors.Wrap(err, "failed to finish the multipart body")
	}

	req, err := c.newRequest(http.MethodPost, path, buf.Bytes(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return c.roundTrip(req)
}
