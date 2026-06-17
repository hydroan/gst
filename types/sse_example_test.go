package types_test

import (
	"fmt"
	"io"
	"time"

	"github.com/hydroan/gst/types"
)

// Note: SSE usage:
//   - Use types.Event for event types
//   - Use ctx.Encode() to encode events in stream callbacks
//   - Use the chainable API for all SSE operations:
//     - ctx.SSE().Stream(fn) - Start a stream
//     - ctx.SSE().WithInterval(duration).Stream(fn) - Stream with interval
//     - ctx.SSE().Done() - Send [DONE] marker

// ExampleServiceContext_SSE_stream demonstrates using the chainable API for streaming.
func ExampleServiceContext_SSE_stream() {
	var sc *types.ServiceContext

	// Stream events using the chainable API
	sc.SSE().Stream(func(w io.Writer) bool {
		_ = sc.Encode(w, types.Event{
			Event: "message",
			Data:  time.Now().String(),
		})
		return true
	})
}

// ExampleServiceContext_SSE_streamWithInterval demonstrates using the chainable API for interval-based streaming.
func ExampleServiceContext_SSE_streamWithInterval() {
	var sc *types.ServiceContext

	// Stream with interval using the chainable API
	counter := 0
	sc.SSE().WithInterval(1 * time.Second).Stream(func(w io.Writer) bool {
		counter++

		// Send event
		_ = sc.Encode(w, types.Event{
			ID:    fmt.Sprintf("event-%d", counter),
			Event: "message",
			Data: map[string]any{
				"number":  counter,
				"message": fmt.Sprintf("Event %d of 3", counter),
			},
		})

		// Return true to continue (will wait 1 second), false to stop
		// This will send 3 events: at 0s, 1s, 2s, then stop
		return counter < 3
	})

	// Send [DONE] marker if required by your protocol
	_ = sc.SSE().Done()
}

// ExampleServiceContext_SSE_done demonstrates using the chainable API to send a [DONE] marker.
func ExampleServiceContext_SSE_done() {
	var sc *types.ServiceContext

	// Send [DONE] marker using the chainable API
	_ = sc.SSE().Done()
}

// ExampleServiceContext_SSE_completeFlow demonstrates a complete SSE flow using the chainable API.
// This pattern is commonly used in AI chat completions and similar streaming APIs.
func ExampleServiceContext_SSE_completeFlow() {
	var sc *types.ServiceContext

	// Stream data chunks using the chainable API
	sc.SSE().Stream(func(w io.Writer) bool {
		// Send chat completion chunks
		_ = sc.Encode(w, types.Event{
			Data: map[string]any{
				"content": "Hello",
			},
		})
		// Continue streaming...
		return true
	})

	// Send [DONE] marker to indicate stream completion
	// This is required by protocols like OpenAI's chat completion API
	_ = sc.SSE().Done()
}
