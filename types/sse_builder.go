package types

import (
	"io"
	"time"

	"github.com/hydroan/gst/internal/sse"
)

// SSEBuilder provides a fluent interface for building and sending SSE events.
// It supports chaining methods to configure and send SSE events or streams.
type SSEBuilder struct {
	ctx      *ServiceContext
	interval time.Duration
}

// SSE returns a new SSE builder for chaining SSE operations.
// This provides a fluent interface for sending SSE events and streams.
//
// Example usage:
//
//	// Start a stream
//	ctx.SSE().Stream(func(w io.Writer) bool {
//	    _ = ctx.Encode(w, types.Event{Data: "chunk"})
//	    return true
//	})
//
//	// Stream with interval
//	ctx.SSE().WithInterval(1*time.Second).Stream(func(w io.Writer) bool {
//	    _ = ctx.Encode(w, types.Event{Data: "chunk"})
//	    return true
//	})
//
//	// Send [DONE] marker
//	_ = ctx.SSE().Done()
func (sc *ServiceContext) SSE() *SSEBuilder {
	if sc == nil {
		return nil
	}
	return &SSEBuilder{ctx: sc}
}

// Stream starts a Server-Sent Events stream.
// The provided function will be called repeatedly until it returns false.
// The stream will automatically stop if:
//   - The function returns false
//   - The request context is canceled (timeout, client disconnect, etc.)
//   - An error occurs while writing to the client
//
// Note: This method does NOT automatically send a [DONE] marker when the stream ends.
// If your protocol requires a [DONE] marker (e.g., AI chat completions), you must
// manually call Done() after Stream() returns.
//
// Example:
//
//	ctx.SSE().Stream(func(w io.Writer) bool {
//	    _ = ctx.Encode(w, types.Event{Data: "chunk"})
//	    return true // Continue streaming
//	})
//	// Send [DONE] marker if required by your protocol
//	_ = ctx.SSE().Done()
func (b *SSEBuilder) Stream(fn func(io.Writer) bool) {
	if b == nil || b.ctx == nil {
		return
	}

	if b.interval > 0 {
		// Stream with interval
		streamSSEWithInterval(b.ctx, b.interval, fn)
	} else {
		// Regular stream
		streamSSE(b.ctx, fn)
	}
}

// WithInterval sets the time interval between events in a stream.
// This must be called before Stream() when using interval-based streaming.
//
// Example:
//
//	ctx.SSE().WithInterval(1*time.Second).Stream(func(w io.Writer) bool {
//	    _ = ctx.Encode(w, types.Event{Data: "chunk"})
//	    return true
//	})
func (b *SSEBuilder) WithInterval(duration time.Duration) *SSEBuilder {
	if b == nil {
		return b
	}
	b.interval = duration
	return b
}

// Done sends a [DONE] marker to indicate the end of an SSE stream.
// This is commonly used in AI chat completions and similar streaming APIs.
//
// Note: This method should be called AFTER Stream() returns,
// if your protocol requires a [DONE] marker. Standard SSE streams do not require this marker
// and will naturally end when the connection closes.
//
// Example:
//
//	ctx.SSE().Stream(func(w io.Writer) bool {
//	    // Send data...
//	    return true
//	})
//	// Send [DONE] marker to indicate stream completion
//	_ = ctx.SSE().Done()
//
// Returns:
//   - error: Any error that occurred during encoding
func (b *SSEBuilder) Done() error {
	if b == nil || b.ctx == nil {
		return nil
	}
	return sendSSEDone(b.ctx)
}

// streamSSE starts a Server-Sent Events stream.
// This is an internal method used by SSEBuilder.
func streamSSE(sc *ServiceContext, fn func(io.Writer) bool) {
	if sc == nil || sc.ginCtx == nil {
		return
	}
	sse.StreamSSE(sc.Context(), sc.ginCtx.Writer, sc.ginCtx.Stream, fn)
}

// streamSSEWithInterval starts a Server-Sent Events stream with a fixed interval between events.
// This is an internal method used by SSEBuilder.
func streamSSEWithInterval(sc *ServiceContext, interval time.Duration, fn func(io.Writer) bool) {
	if sc == nil || sc.ginCtx == nil {
		return
	}
	sse.StreamSSEWithInterval(sc.Context(), sc.ginCtx.Writer, sc.ginCtx.Stream, interval, fn)
}

// sendSSEDone sends a [DONE] marker to indicate the end of an SSE stream.
// This is an internal method used by SSEBuilder.
func sendSSEDone(sc *ServiceContext) error {
	if sc == nil || sc.ginCtx == nil {
		return nil
	}
	return sse.SendSSEDone(sc.ginCtx.Writer)
}
