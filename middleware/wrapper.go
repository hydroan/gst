package middleware

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	gstotel "github.com/hydroan/gst/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
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
// Example:
//
//	wrappedLogger := middlewareWrapper("logger", Logger())
//	wrappedAuth := middlewareWrapper("jwt-auth", JWT())
//	router.Use(wrappedLogger, wrappedAuth)
func middlewareWrapper(name string, middleware gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip tracing if OTEL is not enabled
		if !gstotel.IsEnabled() {
			middleware(c)
			return
		}

		// Use the canonical gst operation name for middleware spans.
		spanName := gstotel.OperationSpanName("middleware", name)

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
			// Set span attributes
			span.SetAttributes(
				attribute.String("middleware.name", name),
				attribute.String("http.method", c.Request.Method),
				attribute.String("http.path", c.Request.URL.Path),
				attribute.String("http.route", c.FullPath()),
			)

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
			attrs := make([]attribute.KeyValue, 0, 6)
			attrs = append(
				attrs,
				attribute.Int64("middleware.duration_ms", duration.Milliseconds()),
				attribute.Int64("middleware.duration_ns", duration.Nanoseconds()),
				attribute.Int("http.status_code", status),
			)

			// Check if middleware caused any errors (based on response status)
			if status >= 400 {
				span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", status))
				attrs = append(attrs, attribute.Bool("middleware.error", true))
			} else {
				span.SetStatus(codes.Ok, "")
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
