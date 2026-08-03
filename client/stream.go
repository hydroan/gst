package client

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/types"
)

// StreamCallback handles one SSE event; returning an error stops the stream.
type StreamCallback func(event types.Event) error

// Stream sends the request and consumes the response as a Server-Sent Events
// stream, pairing the framework's SSE responses. A JSON answer on a stream
// endpoint is parsed as the regular envelope: a rejection surfaces as *Error,
// a success returns nil without events.
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

// parseSSEStream parses a Server-Sent Events stream and calls the callback
// for each event. A "[DONE]" data marker ends the stream.
func parseSSEStream(body io.Reader, callback StreamCallback) error {
	scanner := bufio.NewScanner(body)
	event := types.Event{}
	var dataLines []string

	for scanner.Scan() {
		line := scanner.Text()

		// An empty line closes the current event.
		if line == "" {
			if len(dataLines) > 0 {
				event.Data = strings.Join(dataLines, "\n")
				if err := callback(event); err != nil {
					return err
				}
				event = types.Event{}
				dataLines = nil
			}
			continue
		}

		switch {
		case strings.HasPrefix(line, "id:"):
			event.ID = strings.TrimSpace(line[3:])
		case strings.HasPrefix(line, "event:"):
			event.Event = strings.TrimSpace(line[6:])
		case strings.HasPrefix(line, "retry:"):
			if retry, err := parseInt(strings.TrimSpace(line[6:])); err == nil {
				event.Retry = retry
			}
		case strings.HasPrefix(line, "data:"):
			data := strings.TrimSpace(line[5:])
			if data == "[DONE]" {
				return nil
			}
			dataLines = append(dataLines, data)
		}
		// Comment lines (starting with ":") are ignored.
	}
	if err := scanner.Err(); err != nil {
		return errors.Wrap(err, "failed to read the stream")
	}

	// Deliver the last event when the stream ends without a trailing blank line.
	if len(dataLines) > 0 {
		event.Data = strings.Join(dataLines, "\n")
		if err := callback(event); err != nil {
			return err
		}
	}
	return nil
}

// parseInt parses an integer string.
func parseInt(s string) (int, error) {
	var result int
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}
