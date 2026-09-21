package middleware

import (
	"bufio"
	"context"
	"maps"
	"net"
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	internalmiddleware "github.com/hydroan/gst/internal/middleware"
	"github.com/hydroan/gst/internal/response"
	"go.uber.org/zap"
)

// Timeout returns a middleware that puts a deadline on the request context and
// answers 504 when the rest of the chain has not started its response by then.
//
// The deadline is cooperative. The chain runs to completion on the request
// goroutine, and code that honors the request context (database, cache and
// outbound calls) stops once the deadline passes. A response the chain starts
// writing before the deadline goes out as written; one it would only start
// after the deadline is dropped, together with the headers the chain set, and
// the 504 envelope is written in its place once the chain returns, so a
// handler that ignores the context delays the 504 until it returns. The chain
// never runs on a goroutine of its own: gin takes the context back for the
// next request as soon as this one returns, and a chain still running on it
// would read and write another request.
//
// Streaming routes are left alone, since a stream legitimately outlives any
// request timeout.
//
// Example:
//
//	// Set 30 second timeout for all requests
//	router.Use(middleware.Timeout(30 * time.Second))
//
//	// Set 5 second timeout
//	router.Use(middleware.Timeout(5 * time.Second))
func Timeout(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		// A streaming response legitimately outlives any request timeout;
		// cutting it down here would end every stream at the deadline.
		if internalmiddleware.IsStreamingRequest(c) {
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)

		header := c.Writer.Header().Clone()
		writer := &deadlineWriter{ResponseWriter: c.Writer, ctx: ctx}
		func() {
			// Restored on a panic too, so Recovery answers on the real writer.
			defer func() { c.Writer = writer.ResponseWriter }()
			c.Writer = writer
			c.Next()
		}()

		if writer.started || !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return
		}
		// The chain started no response before the deadline: drop the headers
		// it set along with what it wrote, and answer 504 instead.
		clear(c.Writer.Header())
		maps.Copy(c.Writer.Header(), header)
		zap.S().Warnw(
			"request timeout",
			"path", c.Request.URL.Path,
			"method", c.Request.Method,
			"timeout", timeout,
		)
		response.Abort(c, http.StatusGatewayTimeout, "request timeout")
	}
}

// deadlineWriter settles a response on its first write: a response that
// starts before the deadline goes through to the client, and one that would
// only start after it is dropped, so Timeout can answer 504 in its place.
type deadlineWriter struct {
	gin.ResponseWriter

	ctx      context.Context
	started  bool
	timedOut bool
}

// admit reports whether a write may reach the client, settling the response
// on the first call.
func (w *deadlineWriter) admit() bool {
	if !w.started && !w.timedOut {
		if errors.Is(w.ctx.Err(), context.DeadlineExceeded) {
			w.timedOut = true
		} else {
			w.started = true
		}
	}
	return w.started
}

func (w *deadlineWriter) Write(data []byte) (int, error) {
	if !w.admit() {
		return 0, http.ErrHandlerTimeout
	}
	return w.ResponseWriter.Write(data)
}

func (w *deadlineWriter) WriteString(s string) (int, error) {
	if !w.admit() {
		return 0, http.ErrHandlerTimeout
	}
	return w.ResponseWriter.WriteString(s)
}

func (w *deadlineWriter) WriteHeaderNow() {
	if w.admit() {
		w.ResponseWriter.WriteHeaderNow()
	}
}

func (w *deadlineWriter) Flush() {
	if w.admit() {
		w.ResponseWriter.Flush()
	}
}

func (w *deadlineWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if !w.admit() {
		return nil, nil, http.ErrHandlerTimeout
	}
	return w.ResponseWriter.Hijack()
}
