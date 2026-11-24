package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	gstotel "github.com/hydroan/gst/internal/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// A middleware span's two attribute batches are sized to their worst case
// once per middleware per traced request, so the hot path never regrows
// them; the worst-case test keeps each count honest when an attribute is
// added.
const (
	// middlewareStartAttrCap is the most attributes the batch a middleware
	// span starts with carries: the middleware's name, the request's method
	// and path, and the route it matched.
	middlewareStartAttrCap = 4
	// middlewareOutcomeAttrCap is the most attributes the outcome batch
	// carries: the duration in milliseconds and in nanoseconds, the status
	// code, the error flag and the service name.
	middlewareOutcomeAttrCap = 5
)

// middlewareWrapper wraps any gin middleware with OTEL tracing capabilities.
// It creates a span for the middleware execution and records performance metrics.
//
// The span covers exactly what the middleware itself runs. A middleware that
// only acts before the handler returns without calling c.Next(): gin carries
// the chain on by itself, the values the middleware attached to the request
// context stay attached, and its span ends before the next handler starts. A
// middleware that has to observe the response calls c.Next(), and its span
// then covers the downstream handlers as well.
//
// Parameters:
//   - name: The name of the middleware for tracing identification
//   - middleware: The gin.HandlerFunc to be wrapped
//
// Returns:
//   - A new gin.HandlerFunc with tracing capabilities
//
// Register and RegisterAuth wrap every middleware they keep this way, under the
// name of the function that built it:
//
//	wrapped := middlewareWrapper(getFunctionName(middleware), middleware)
func middlewareWrapper(name string, middleware gin.HandlerFunc) gin.HandlerFunc {
	// The canonical gst operation name of the middleware's span, built once
	// here, as the middleware is wrapped, and shared by every request it
	// serves.
	spanName := gstotel.OperationSpanName("middleware", name)

	return func(c *gin.Context) {
		// Skip tracing if OTEL is not enabled
		if !gstotel.IsEnabled() {
			middleware(c)
			return
		}

		// Start new span for middleware execution under the HTTP request span.
		originalCtx := c.Request.Context()
		parentCtx := gstotel.RequestRootContext(originalCtx)
		ctx, span := gstotel.StartSpan(parentCtx, spanName)
		defer span.End()

		// The middleware runs with its own span current. Whatever it attaches
		// to the request context — an authenticated session, a tenant scope —
		// must outlive it, so the context is kept on return and only the span
		// is swapped back to the parent, so later handlers nest under the
		// request root rather than under a span that has ended.
		c.Request = c.Request.WithContext(ctx)
		defer func() {
			c.Request = c.Request.WithContext(trace.ContextWithSpan(c.Request.Context(), trace.SpanFromContext(parentCtx)))
		}()

		recording := gstotel.IsSpanRecording(span)
		var start time.Time
		if recording {
			// The span describes the request it serves the way the request
			// root span does (see tracing): by the names of the
			// OpenTelemetry HTTP conventions, the route left off a request
			// that matched none.
			attrs := make([]attribute.KeyValue, 0, middlewareStartAttrCap)
			attrs = append(
				attrs,
				attribute.String("middleware.name", name),
				semconv.HTTPRequestMethodKey.String(c.Request.Method),
				semconv.URLPath(c.Request.URL.Path),
			)
			if route := c.FullPath(); route != "" {
				attrs = append(attrs, semconv.HTTPRoute(route))
			}
			span.SetAttributes(attrs...)

			// Record start time
			start = time.Now()
		}

		// Execute the wrapped middleware
		middleware(c)

		if recording {
			// Performance: batch all completion attributes into one SetAttributes
			// call; every extra call locks the span, grows the attribute slice,
			// and re-runs deduplication. This runs once per middleware per traced
			// request, so when adding attributes, extend this batch instead of
			// adding SetAttributes calls.
			duration := time.Since(start)
			status := c.Writer.Status()
			attrs := make([]attribute.KeyValue, 0, middlewareOutcomeAttrCap)
			attrs = append(
				attrs,
				attribute.Int64("middleware.duration_ms", duration.Milliseconds()),
				attribute.Int64("middleware.duration_ns", duration.Nanoseconds()),
				semconv.HTTPResponseStatusCode(status),
			)

			// A middleware answering a server error failed; one refusing the
			// request, 4xx, did its job and leaves the status Unset, the way
			// the request root span and the gRPC line's spans read (see
			// tracing), and Ok is never set.
			if status >= http.StatusInternalServerError {
				span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", status))
				attrs = append(attrs, attribute.Bool("middleware.error", true))
			} else {
				attrs = append(attrs, attribute.Bool("middleware.error", false))
			}

			// Add the service name attribute cached at otel Init.
			if serviceName := gstotel.ServiceNameAttr(); serviceName.Valid() {
				attrs = append(attrs, serviceName)
			}
			span.SetAttributes(attrs...)
		}
	}
}
