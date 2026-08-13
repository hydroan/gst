package client

import (
	"bufio"
	"bytes"
	"io"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/sse"
)

// maxSSELineLength bounds a single SSE line. The default bufio.Scanner limit
// of 64KB is close to realistic event sizes; a malformed or hostile stream
// should fail the scan instead of growing the buffer without bound.
const maxSSELineLength = 1024 * 1024

// StreamCallback handles one SSE event; returning an error stops the stream.
type StreamCallback func(event sse.Event) error

// Stream sends the request and consumes the response as a Server-Sent Events
// stream, pairing the framework's SSE responses. The stream ends when the
// server closes the connection or the callback returns an error. A JSON
// answer on a stream endpoint is parsed as the regular envelope: a rejection
// surfaces as *Error, a success returns nil without events.
func (c *Client) Stream(method, path string, payload any, callback StreamCallback) error {
	if callback == nil {
		return errors.New("callback cannot be nil")
	}

	req, err := c.newRequest(method, path, payload, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	httpRsp, err := c.httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "failed to send the request")
	}
	defer httpRsp.Body.Close()

	if !strings.HasPrefix(httpRsp.Header.Get("Content-Type"), "text/event-stream") {
		body, err := io.ReadAll(httpRsp.Body)
		if err != nil {
			return errors.Wrap(err, "failed to read the response body")
		}
		_, envErr := parseEnvelope(httpRsp, body)
		return envErr
	}
	return parseSSEStream(httpRsp.Body, callback)
}

// parseSSEStream reads a Server-Sent Events stream following the WHATWG
// processing model and calls the callback for each dispatched event:
//
//   - a blank line dispatches the buffered event; events without data lines
//     are not dispatched (their event type is still discarded);
//   - the last seen event ID sticks across events until replaced, as it does
//     in a browser EventSource; IDs containing NUL are ignored;
//   - retry only accepts all-digit values and rides along on every event;
//   - comment lines and unknown fields are ignored;
//   - an event still buffered when the stream ends is discarded.
func parseSSEStream(body io.Reader, callback StreamCallback) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), maxSSELineLength)
	scanner.Split(scanSSELines)

	var (
		dataLines   []string
		eventType   string
		lastEventID string
		retry       int
		firstLine   = true
	)
	for scanner.Scan() {
		line := scanner.Text()
		if firstLine {
			// A leading byte order mark is part of the encoding, not the data.
			line = strings.TrimPrefix(line, "\uFEFF")
			firstLine = false
		}

		if line == "" {
			if len(dataLines) > 0 {
				event := sse.Event{
					ID:    lastEventID,
					Event: eventType,
					Retry: retry,
					Data:  strings.Join(dataLines, "\n"),
				}
				if err := callback(event); err != nil {
					return err
				}
			}
			dataLines = nil
			eventType = ""
			continue
		}

		if strings.HasPrefix(line, ":") {
			continue
		}

		field, value, hasColon := strings.Cut(line, ":")
		if hasColon {
			// Only a single leading space belongs to the separator.
			value = strings.TrimPrefix(value, " ")
		}
		switch field {
		case "id":
			if !strings.Contains(value, "\x00") {
				lastEventID = value
			}
		case "event":
			eventType = value
		case "retry":
			if isAllDigits(value) {
				if v, err := strconv.Atoi(value); err == nil {
					retry = v
				}
			}
		case "data":
			dataLines = append(dataLines, value)
		}
	}
	if err := scanner.Err(); err != nil {
		return errors.Wrap(err, "failed to read the stream")
	}
	return nil
}

// scanSSELines is a bufio.SplitFunc for the three line terminators the SSE
// format allows: CRLF, a lone LF, and a lone CR.
func scanSSELines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		if data[i] == '\n' {
			return i + 1, data[:i], nil
		}
		switch {
		case i+1 < len(data):
			if data[i+1] == '\n' {
				return i + 2, data[:i], nil // CRLF
			}
			return i + 1, data[:i], nil // lone CR
		case atEOF:
			return i + 1, data[:i], nil // CR at the very end
		default:
			return 0, nil, nil // need more data to tell CR from CRLF
		}
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// isAllDigits reports whether s is non-empty and consists of ASCII digits
// only, the shape the SSE retry field requires.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
