package types

import (
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/sse"
)

// SSE turns the response into a Server-Sent Events stream and runs fn with
// the live connection.
//
// The framework owns the connection lifecycle: it clears the server's
// per-request deadlines so the stream outlives the global WriteTimeout,
// writes and flushes the SSE response headers, sends keep-alive comment
// frames until fn returns, and invalidates the connection afterwards. fn
// blocks until the stream is over; a callback that waits for events must
// select on conn.Context().Done() to notice the client disconnecting.
//
// The error is fn's own error, or the setup failure that prevented streaming
// (reported before anything was written, so it still surfaces as a regular
// error response).
//
// Example:
//
//	return nil, ctx.SSE(func(conn *sse.Conn) error {
//		for {
//			select {
//			case <-conn.Context().Done():
//				return nil
//			case event := <-events:
//				if err := conn.Send(event); err != nil {
//					return err
//				}
//			}
//		}
//	})
func (sc *ServiceContext) SSE(fn func(conn *sse.Conn) error, opts ...sse.Option) error {
	if sc == nil || sc.ginCtx == nil {
		return errors.New("service context carries no HTTP response to stream on")
	}
	return sse.Serve(sc.ginCtx.Writer, sc.ginCtx.Request, fn, opts...)
}
