// Package sse provides Server-Sent Events (SSE) implementation for Go.
// SSE is a technology where a browser receives automatic updates from a server via HTTP connection.
// The Server-Sent Events EventSource API is standardized as part of HTML5 by the W3C.
//
// This package is inspired by github.com/manucorporat/sse and provides
// encoding functionality for SSE events compatible with the Gin framework.
package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// doneMarker is a special marker sent at the end of an SSE stream to indicate completion.
// This is commonly used in AI chat completions and similar streaming APIs.
const doneMarker = "[DONE]"

// Event represents a Server-Sent Event.
// Each event can have an optional ID, event type, retry interval, and data.
type Event struct {
	// ID is an optional event identifier. If set, the client will set the Last-Event-ID header
	// on reconnection, allowing the server to resume from where it left off.
	ID string

	// Event is an optional event type. If set, the client will dispatch an event with this type
	// instead of the default "message" event.
	Event string

	// Retry is an optional retry interval in milliseconds. If set, the client will wait this
	// many milliseconds before attempting to reconnect after a connection is lost.
	Retry int

	// Data is the event payload. It can be a primitive type (string, int, float) or a complex
	// type (map, struct, slice). Complex types will be JSON-encoded.
	Data any
}

// StreamFunc is a function type for streaming SSE events.
// It receives the writer and should return false to stop streaming.
type StreamFunc func(io.Writer) bool

// StreamCallback is a function type that starts streaming with the provided function.
// This is typically gin.Context.Stream or a wrapper around it.
// The signature matches gin.Context.Stream: func(step func(io.Writer) bool) bool
type StreamCallback func(func(io.Writer) bool) bool

// Encode writes an SSE event to the given writer.
// The event is formatted according to the SSE specification:
//   - Fields are written in recommended order: id, event, retry, data
//   - Each field is written as "field: value\n"
//   - Multiple data fields are concatenated (for multi-line data)
//   - Events are separated by a blank line (\n\n)
//
// If Data is a complex type (map, struct, slice), it will be JSON-encoded.
// If Data is a primitive type, it will be converted to string.
// If Data is nil, no data field will be written.
//
// Example output:
//
//	id: 124
//	event: message
//	retry: 3000
//	data: some data
//	data: more data
//
//	id: 125
//	event: message
//	data: {"user":"manu","date":1431540810,"content":"hi!"}
//
//	data: [DONE]
func Encode(w io.Writer, event Event) error {
	// Write fields in recommended order: id, event, retry, data
	if len(event.ID) > 0 {
		if _, err := fmt.Fprintf(w, "id: %s\n", escape(event.ID)); err != nil {
			return err
		}
	}

	if len(event.Event) > 0 {
		if _, err := fmt.Fprintf(w, "event: %s\n", escape(event.Event)); err != nil {
			return err
		}
	}

	if event.Retry > 0 {
		if _, err := fmt.Fprintf(w, "retry: %d\n", event.Retry); err != nil {
			return err
		}
	}

	// Handle data field
	// Note: Even empty objects like {} should be sent as data: {}
	if event.Data != nil {
		data, err := formatData(event.Data)
		if err != nil {
			return err
		}

		// Split data by newlines and write each line as a separate "data:" field
		// This is required by the SSE specification for multi-line data
		lines := strings.SplitSeq(data, "\n")
		for line := range lines {
			if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
				return err
			}
		}
	}

	// End the event with a blank line (two newlines: one for the last field, one for event separator)
	_, err := fmt.Fprint(w, "\n")
	return err
}

// formatData converts the data to a string representation.
// Primitive types are converted directly, complex types are JSON-encoded.
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
		// For complex types, use JSON encoding
		jsonData, err := json.Marshal(data)
		if err != nil {
			return "", fmt.Errorf("failed to marshal data: %w", err)
		}
		return string(jsonData), nil
	}
}

// EncodeDone sends a [DONE] marker to indicate the end of an SSE stream.
// This is commonly used in AI chat completions and similar streaming APIs.
//
// Example:
//
//	EncodeDone(w)  // Sends: data: [DONE]\n\n
func EncodeDone(w io.Writer) error {
	_, err := fmt.Fprintf(w, "data: %s\n\n", doneMarker)
	return err
}

// escape escapes special characters in SSE field values.
// According to the SSE specification, newlines and carriage returns must be escaped.
func escape(s string) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	return s
}

// SendSSE sends a single Server-Sent Events (SSE) event.
// This function sets the appropriate headers for SSE and writes the event to the response.
// The response is automatically flushed to ensure immediate delivery.
//
// Note: This function sends a single event, not a stream. If you need to send a [DONE] marker
// after this event (e.g., for AI chat completions), you should call SendSSEDone() after this function.
//
// Parameters:
//   - w: HTTP response writer
//   - event: SSE event to send
//
// Returns:
//   - error: Any error that occurred during encoding
func SendSSE(w http.ResponseWriter, event Event) error {
	setHeaders(w)

	if err := Encode(w, event); err != nil {
		return err
	}

	// Flush to ensure immediate delivery
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	return nil
}

// SendSSEDone sends a [DONE] marker to indicate the end of an SSE stream.
// This is commonly used in AI chat completions and similar streaming APIs.
//
// Note: This function should be called AFTER StreamSSE() or StreamSSEWithInterval() returns,
// if your protocol requires a [DONE] marker. Standard SSE streams do not require this marker
// and will naturally end when the connection closes.
//
// Parameters:
//   - w: HTTP response writer
//
// Returns:
//   - error: Any error that occurred during encoding
func SendSSEDone(w http.ResponseWriter) error {
	setHeaders(w)

	if err := EncodeDone(w); err != nil {
		return err
	}

	// Flush to ensure immediate delivery
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	return nil
}

// StreamSSE starts a Server-Sent Events stream.
// The provided function will be called repeatedly until it returns false.
// The stream will automatically stop if:
//   - The function returns false
//   - The request context is canceled (timeout, client disconnect, etc.)
//   - An error occurs while writing to the client
//
// Note: This function does NOT automatically send a [DONE] marker when the stream ends.
// If your protocol requires a [DONE] marker (e.g., AI chat completions), you must
// manually call SendSSEDone() after StreamSSE() returns.
//
// Parameters:
//   - ctx: Context for cancellation support
//   - w: HTTP response writer for setting headers
//   - stream: Function that starts the stream (typically gin.Context.Stream)
//   - fn: Function that sends events. Returns false to stop streaming.
//     The function receives the writer and should check context cancellation if needed.
func StreamSSE(ctx context.Context, w http.ResponseWriter, stream StreamCallback, fn StreamFunc) {
	setHeaders(w)

	stream(func(w io.Writer) bool {
		// Check if context is canceled (timeout, client disconnect, etc.)
		select {
		case <-ctx.Done():
			return false
		default:
		}

		// Call user function
		shouldContinue := fn(w)

		// Flush after each event to ensure immediate delivery
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}

		return shouldContinue
	})
}

// StreamSSEWithInterval starts a Server-Sent Events stream with a fixed interval between events.
// The provided function will be called repeatedly at the specified interval until it returns false.
// The stream will automatically stop if:
//   - The function returns false
//   - The request context is canceled (timeout, client disconnect, etc.)
//   - An error occurs while writing to the client
//
// Note: This function does NOT automatically send a [DONE] marker when the stream ends.
// If your protocol requires a [DONE] marker (e.g., AI chat completions), you must
// manually call SendSSEDone() after StreamSSEWithInterval() returns.
//
// Parameters:
//   - ctx: Context for cancellation support
//   - w: HTTP response writer for setting headers
//   - stream: Function that starts the stream (typically gin.Context.Stream)
//   - interval: Time interval between events
//   - fn: Function that sends events. Returns false to stop streaming.
//     The function receives the writer and should check context cancellation if needed.
func StreamSSEWithInterval(ctx context.Context, w http.ResponseWriter, stream StreamCallback, interval time.Duration, fn StreamFunc) {
	setHeaders(w)

	stream(func(w io.Writer) bool {
		// Check if context is canceled (timeout, client disconnect, etc.)
		select {
		case <-ctx.Done():
			return false
		default:
		}

		// Call user function
		if !fn(w) {
			return false
		}

		// Flush after each event to ensure immediate delivery
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}

		// Sleep with context cancellation support
		select {
		case <-ctx.Done():
			return false
		case <-time.After(interval):
			return true
		}
	})
}

// setHeaders sets the standard SSE headers on the response writer.
func setHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering
}
