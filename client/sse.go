package client

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/sse"
)

// maxSSELineLength bounds a single SSE line. The default bufio.Scanner limit
// of 64KB is close to realistic event sizes; a malformed or hostile stream
// should fail the scan instead of growing the buffer without bound.
const maxSSELineLength = 1024 * 1024

// EventStream is the client's end of a Server-Sent Events response, the
// stream a model's SSE action answers with: Recv returns its events one by
// one. The caller closes it.
type EventStream struct {
	body    io.Closer
	scanner *bufio.Scanner

	// What the stream carries from one event to the next: the last event ID
	// and the retry interval stick until replaced, as they do in a browser
	// EventSource, and started tells the first line, whose byte order mark is
	// not data, from the rest.
	lastEventID string
	retry       int
	started     bool
}

// SSE sends the request on ctx and returns the response as a Server-Sent
// Events stream, pairing the framework's SSE action. The stream ends when the
// server closes the connection, Recv then answering io.EOF, and when ctx
// ends, Recv then failing with the context's error. A JSON answer on a stream
// endpoint is parsed as the regular envelope: a rejection surfaces as *Error,
// and a success returns a stream carrying no event.
func (c *Client) SSE(ctx context.Context, method, path string, payload any, opts ...RequestOption) (*EventStream, error) {
	req, err := c.newRequest(ctx, method, path, payload, opts)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")

	httpRsp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to send the request")
	}
	if strings.HasPrefix(httpRsp.Header.Get("Content-Type"), "text/event-stream") {
		return newEventStream(httpRsp.Body), nil
	}
	defer httpRsp.Body.Close()

	body, err := io.ReadAll(httpRsp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read the response body")
	}
	if _, err = parseEnvelope(httpRsp, body); err != nil {
		return nil, err
	}
	return newEventStream(io.NopCloser(bytes.NewReader(nil))), nil
}

// newEventStream returns the stream of the events body carries.
func newEventStream(body io.ReadCloser) *EventStream {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), maxSSELineLength)
	scanner.Split(scanSSELines)
	return &EventStream{body: body, scanner: scanner}
}

// Recv returns the next event of the stream, read following the WHATWG
// processing model:
//
//   - a blank line dispatches the buffered event; events without data lines
//     are not dispatched (their event type is still discarded);
//   - the last seen event ID sticks across events until replaced, as it does
//     in a browser EventSource; IDs containing NUL are ignored;
//   - retry only accepts all-digit values and rides along on every event;
//   - comment lines and unknown fields are ignored;
//   - an event still buffered when the stream ends is discarded.
//
// It answers io.EOF once the server has ended the stream.
func (s *EventStream) Recv() (sse.Event, error) {
	var (
		dataLines []string
		eventType string
	)
	for s.scanner.Scan() {
		line := s.scanner.Text()
		if !s.started {
			// A leading byte order mark is part of the encoding, not the data.
			line = strings.TrimPrefix(line, "\uFEFF")
			s.started = true
		}

		if line == "" {
			if len(dataLines) > 0 {
				return sse.Event{
					ID:    s.lastEventID,
					Event: eventType,
					Retry: s.retry,
					Data:  strings.Join(dataLines, "\n"),
				}, nil
			}
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
				s.lastEventID = value
			}
		case "event":
			eventType = value
		case "retry":
			if isAllDigits(value) {
				if v, err := strconv.Atoi(value); err == nil {
					s.retry = v
				}
			}
		case "data":
			dataLines = append(dataLines, value)
		}
	}
	if err := s.scanner.Err(); err != nil {
		return sse.Event{}, errors.Wrap(err, "failed to read the stream")
	}
	return sse.Event{}, io.EOF
}

// Close ends the stream from the client's side and releases its connection.
func (s *EventStream) Close() error {
	return errors.Wrap(s.body.Close(), "failed to close the stream")
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
