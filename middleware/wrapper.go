package middleware

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/config"
	gstotel "github.com/hydroan/gst/provider/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// middlewareWrapper wraps any gin middleware with OTEL tracing capabilities.
// It creates a span for the middleware execution and records performance metrics.
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

		// Update request context with the new span context
		c.Request = c.Request.WithContext(ctx)
		defer func() {
			c.Request = c.Request.WithContext(originalCtx)
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
			// Record execution duration
			duration := time.Since(start)
			span.SetAttributes(
				attribute.Int64("middleware.duration_ms", duration.Milliseconds()),
				attribute.Int64("middleware.duration_ns", duration.Nanoseconds()),
			)

			// Check if middleware caused any errors (based on response status)
			if c.Writer.Status() >= 400 {
				span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", c.Writer.Status()))
				span.SetAttributes(
					attribute.Int("http.status_code", c.Writer.Status()),
					attribute.Bool("middleware.error", true),
				)
			} else {
				span.SetStatus(codes.Ok, "")
				span.SetAttributes(
					attribute.Int("http.status_code", c.Writer.Status()),
					attribute.Bool("middleware.error", false),
				)
			}

			// Add service name as attribute
			if config.App.OTEL.ServiceName != "" {
				span.SetAttributes(
					attribute.String("service.name", config.App.OTEL.ServiceName),
				)
			}
		}
	}
}
