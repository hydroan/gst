package middleware

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/config"
	gstotel "github.com/hydroan/gst/provider/otel"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Tracing returns a middleware that handles both trace ID generation and OpenTelemetry tracing
// This middleware combines the functionality of TraceID() and Tracing() middlewares
func Tracing() gin.HandlerFunc {
	return func(c *gin.Context) {
		var traceID, spanID string
		var span trace.Span
		var ctx context.Context

		// If OTEL is enabled, create OpenTelemetry span and use its trace ID
		if gstotel.IsEnabled() {
			// Create span name from HTTP method and route
			spanName := c.Request.Method + " " + c.FullPath()
			if c.FullPath() == "" {
				spanName = c.Request.Method + " " + c.Request.URL.Path
			}

			// Extract upstream trace context before starting the server span.
			parentCtx := extractRequestTraceContext(c.Request.Context(), c.Request.Header)

			// Start new span
			ctx, span = gstotel.StartSpan(parentCtx, spanName, trace.WithSpanKind(trace.SpanKindServer))
			ctx = gstotel.ContextWithRequestRootSpan(ctx)

			// Extract OTEL trace ID and span ID
			spanContext := span.SpanContext()
			if spanContext.HasTraceID() {
				traceID = spanContext.TraceID().String()
				spanID = spanContext.SpanID().String()
			}

			if gstotel.IsSpanRecording(span) {
				attrs := []attribute.KeyValue{
					attribute.String("http.method", c.Request.Method),
					attribute.String("http.url", c.Request.URL.String()),
					attribute.String("http.scheme", c.Request.URL.Scheme),
					attribute.String("http.host", c.Request.Host),
					attribute.String("http.target", c.Request.URL.Path),
					attribute.String("http.route", c.FullPath()),
					attribute.String("http.user_agent", c.Request.UserAgent()),
					attribute.String("http.remote_addr", c.ClientIP()),
				}

				// Add request headers as attributes (selective)
				if contentType := c.Request.Header.Get("Content-Type"); contentType != "" {
					attrs = append(attrs, attribute.String("http.request.content_type", contentType))
				}
				if contentLength := c.Request.Header.Get("Content-Length"); contentLength != "" {
					attrs = append(attrs, attribute.String("http.request.content_length", contentLength))
				}
				span.SetAttributes(attrs...)
			}

			// Store span in context for use in handlers
			c.Set("otel_span", span)
			c.Request = c.Request.WithContext(ctx)

			// Defer span completion
			defer func() {
				if gstotel.IsSpanRecording(span) {
					// Record response attributes
					span.SetAttributes(
						attribute.Int("http.status_code", c.Writer.Status()),
						attribute.Int("http.response.size", c.Writer.Size()),
						attribute.String("http.response.content_type", c.Writer.Header().Get("Content-Type")),
					)

					// Set span status based on HTTP status code
					statusCode := c.Writer.Status()
					if statusCode >= 400 {
						span.SetStatus(codes.Error, "HTTP "+strconv.Itoa(statusCode))
						span.SetAttributes(attribute.Bool("error", true))
					} else {
						span.SetStatus(codes.Ok, "")
					}

					// Record any errors from the request context
					if len(c.Errors) > 0 {
						attrs := make([]attribute.KeyValue, 0, len(c.Errors)+1)
						attrs = append(attrs, attribute.Bool("error", true))
						span.SetStatus(codes.Error, c.Errors.String())
						for i, err := range c.Errors {
							attrs = append(attrs, attribute.String("error."+strconv.Itoa(i), err.Error()))
						}
						span.SetAttributes(attrs...)
					}
				}

				span.End()
			}()
		} else {
			// Fallback to custom ID generation if OTEL is not enabled
			customTraceID := c.Request.Header.Get(consts.HEADER_TRACE_ID)
			customSpanID := util.SpanID()
			if len(customTraceID) == 0 {
				customTraceID = customSpanID
			}
			traceID = customTraceID
			spanID = customSpanID
		}

		// Set trace fields in gin context.
		c.Set(consts.TRACE_ID, traceID)
		c.Set(consts.SPAN_ID, spanID)
		c.Set(consts.SEQ, 0)

		// Set X-Trace-ID header for callers.
		c.Header(consts.HEADER_TRACE_ID, traceID)

		// Add gst trace IDs as span attributes if OTEL is enabled
		if gstotel.IsEnabled() && span != nil {
			recording := gstotel.IsSpanRecording(span)
			var start time.Time
			if recording {
				span.SetAttributes(
					attribute.String(config.App.OTEL.ServiceName+".trace_id", traceID),
					attribute.String(config.App.OTEL.ServiceName+".span_id", spanID),
				)

				// Record start time for duration calculation
				start = time.Now()
				c.Set("request_start_time", start)
			}

			// Process request
			c.Next()

			if recording {
				// Record duration
				duration := time.Since(start)
				span.SetAttributes(attribute.Int64("http.duration_ms", duration.Milliseconds()))
			}
		} else {
			// Process request without tracing
			c.Next()
		}
	}
}

func extractRequestTraceContext(ctx context.Context, header http.Header) context.Context {
	parentCtx := otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(header))
	if trace.SpanContextFromContext(parentCtx).IsValid() {
		return parentCtx
	}

	traceIDValue := strings.TrimSpace(header.Get(consts.HEADER_TRACE_ID))
	if len(traceIDValue) == 0 {
		return parentCtx
	}

	traceID, err := trace.TraceIDFromHex(traceIDValue)
	if err != nil {
		return parentCtx
	}

	spanIDValue := strings.TrimSpace(header.Get(consts.HEADER_SPAN_ID))
	if len(spanIDValue) == 0 {
		spanIDValue = "0000000000000001"
	}
	spanID, err := trace.SpanIDFromHex(spanIDValue)
	if err != nil {
		spanID = trace.SpanID{0, 0, 0, 0, 0, 0, 0, 1}
	}

	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	return trace.ContextWithRemoteSpanContext(parentCtx, spanContext)
}

// GetSpanFromContext retrieves the OpenTelemetry span from Gin context
func GetSpanFromContext(c *gin.Context) trace.Span {
	if span, exists := c.Get("otel_span"); exists {
		if otelSpan, ok := span.(trace.Span); ok {
			return otelSpan
		}
	}
	return trace.SpanFromContext(c.Request.Context())
}

// AddSpanTags adds custom tags to the current span
func AddSpanTags(c *gin.Context, tags map[string]any) {
	span := GetSpanFromContext(c)
	if span != nil && span.IsRecording() {
		gstotel.AddSpanTags(span, tags)
	}
}

// AddSpanEvent adds an event to the current span
func AddSpanEvent(c *gin.Context, name string, attrs ...attribute.KeyValue) {
	span := GetSpanFromContext(c)
	if span != nil && span.IsRecording() {
		gstotel.AddSpanEvent(span, name, attrs...)
	}
}

// RecordError records an error in the current span
func RecordError(c *gin.Context, err error) {
	span := GetSpanFromContext(c)
	if span != nil && span.IsRecording() {
		gstotel.RecordError(span, err)
	}
}
