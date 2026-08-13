// Package sse implements Server-Sent Events (SSE) for the framework.
// SSE is a technology where a browser receives automatic updates from a server
// via a long-lived HTTP connection; the wire format and client processing
// model are standardized in the WHATWG HTML specification.
//
// The package has two layers:
//
//   - sse.go: the wire format. Event is one frame, Encode writes it.
//   - conn.go: the connection lifecycle. Serve prepares an HTTP response for
//     streaming and hands the business a Conn that sends events safely.
package sse

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
)

// Event represents a single Server-Sent Event frame.
//
// A frame with only Comment set is a comment frame: it is ignored by
// EventSource clients and is the standard way to keep a connection alive
// through proxies. The zero value is not a valid frame; Encode rejects it.
type Event struct {
	// Comment is an optional comment line. Comments are invisible to
	// EventSource clients and serve as keep-alive traffic. Multi-line
	// comments are written as one comment line per newline-separated line;
	// carriage returns are rejected.
	Comment string

	// ID is an optional event identifier. If set, the client remembers it and
	// sends it back as the Last-Event-ID header on reconnection, allowing the
	// server to resume from where the client left off. Must not contain
	// newlines, carriage returns, or NUL characters.
	ID string

	// Event is an optional event type. If set, the client dispatches an event
	// of this type instead of the default "message" event. Must not contain
	// newlines or carriage returns.
	Event string

	// Retry is an optional reconnection interval in milliseconds. If positive,
	// the client waits this long before reconnecting after the connection is
	// lost. Zero means "not set"; negative values are rejected.
	Retry int

	// Data is the event payload. Primitive types (string, []byte, numbers,
	// bool) are written verbatim; any other type is JSON-encoded. Multi-line
	// payloads are split into one data line per line, as the wire format
	// requires. Nil writes no data line.
	Data any
}

// validate reports whether the event can be represented on the wire.
//
// Field values live on single wire lines, so a newline or carriage return in
// ID or Event would break the frame apart; the spec additionally tells
// clients to ignore IDs containing NUL. Such values are caller bugs and are
// rejected instead of silently rewritten. An event with no field set at all
// encodes to nothing meaningful and is rejected too.
func (e Event) validate() error {
	if e.Comment == "" && e.ID == "" && e.Event == "" && e.Retry == 0 && e.Data == nil {
		return errors.New("sse: event has no fields set")
	}
	if strings.Contains(e.Comment, "\r") {
		return errors.Newf("sse: comment %q contains a carriage return", e.Comment)
	}
	if strings.ContainsAny(e.ID, "\n\r\x00") {
		return errors.Newf("sse: event id %q contains a newline, carriage return or NUL", e.ID)
	}
	if strings.ContainsAny(e.Event, "\n\r") {
		return errors.Newf("sse: event type %q contains a newline or carriage return", e.Event)
	}
	if e.Retry < 0 {
		return errors.Newf("sse: retry %d is negative", e.Retry)
	}
	return nil
}

// Encode validates the event and writes it as one SSE frame.
//
// Fields are written in the order comment, id, event, retry, data; multi-line
// comments and data are split into one line per field line, and the frame is
// terminated by a blank line. The frame is buffered and written with a single
// Write call, so a failed write never leaves half a frame on the wire.
//
// Example output:
//
//	: keep-alive
//	id: 124
//	event: message
//	retry: 3000
//	data: {"user":"manu","content":"hi!"}
func Encode(w io.Writer, event Event) error {
	if err := event.validate(); err != nil {
		return err
	}

	var buf bytes.Buffer
	if event.Comment != "" {
		for line := range strings.SplitSeq(event.Comment, "\n") {
			buf.WriteString(": ")
			buf.WriteString(line)
			buf.WriteString("\n")
		}
	}
	if event.ID != "" {
		buf.WriteString("id: ")
		buf.WriteString(event.ID)
		buf.WriteString("\n")
	}
	if event.Event != "" {
		buf.WriteString("event: ")
		buf.WriteString(event.Event)
		buf.WriteString("\n")
	}
	if event.Retry > 0 {
		buf.WriteString("retry: ")
		buf.WriteString(strconv.Itoa(event.Retry))
		buf.WriteString("\n")
	}
	if event.Data != nil {
		data, err := formatData(event.Data)
		if err != nil {
			return err
		}
		// Split data by newlines and write each line as a separate "data:"
		// line; the client reassembles them joined by newlines. Note that an
		// empty payload still writes one "data: " line, as required to
		// dispatch events whose payload is the empty string.
		for line := range strings.SplitSeq(data, "\n") {
			buf.WriteString("data: ")
			buf.WriteString(line)
			buf.WriteString("\n")
		}
	}

	// A blank line terminates the frame.
	buf.WriteString("\n")

	if _, err := w.Write(buf.Bytes()); err != nil {
		return errors.Wrap(err, "sse: failed to write event")
	}
	return nil
}

// formatData converts the data to its wire string representation.
// Primitive types are converted directly, everything else is JSON-encoded.
func formatData(data any) (string, error) {
	switch v := data.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%d", v), nil
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v), nil
	case float32, float64:
		return fmt.Sprintf("%g", v), nil
	case bool:
		return strconv.FormatBool(v), nil
	default:
		encoded, err := json.Marshal(data)
		if err != nil {
			return "", errors.Wrap(err, "sse: failed to marshal event data")
		}
		return string(encoded), nil
	}
}
