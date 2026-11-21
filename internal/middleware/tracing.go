package middleware

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/util"
	gstotel "github.com/hydroan/gst/otel"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// The root span's attribute batches are sized to their worst case once, so a
// traced request never regrows a slice; each count is pinned by a worst-case
// test, which is what keeps it honest when an attribute is added.
const (
	// requestSpanAttrCap is the most attributes the request batch carries:
	// the five every request has — its method, path, scheme, server address
	// and client address — plus the port its Host names, its query string,
	// its route, its user agent and the size of its body.
	requestSpanAttrCap = 10
	// responseSpanAttrCap is the most attributes the response batch carries:
	// the status code, the duration, the size of the body, and the error
	// flag of a failed request.
	responseSpanAttrCap = 4
)

// The root span describes the request by the attributes of the OpenTelemetry
// HTTP semantic conventions, built by the semconv package. Headers are not
// among them: the conventions leave capturing one to an explicit choice, and
// a header attribute is an array of strings, which costs a traced request
// several times what a scalar attribute does.
//
// The scheme of a request is one of two values, so both attributes are built
// once.
var (
	schemeHTTP  = semconv.URLScheme("http")
	schemeHTTPS = semconv.URLScheme("https")
)

// tracing returns the middleware that opens the request root span, stamps
// the trace id on the request context as the identity of this execution, and
// publishes the trace and span ids to the gin context and the response
// headers, falling back to generated ids when OpenTelemetry is disabled.
func tracing() gin.HandlerFunc {
	return func(c *gin.Context) {
		var traceID, spanID string
		var span trace.Span
		var ctx context.Context
		var recording bool
		var start time.Time

		// If OTEL is enabled, create OpenTelemetry span and use its trace ID
		if gstotel.IsEnabled() {
			// The span is named after the method and the route pattern gin
			// matched, so every request to one route shares one name. A
			// request no route matched is named after its method alone, the
			// name the OpenTelemetry HTTP semantic conventions give it: its
			// path is whatever the client sent, and a name per path would
			// hand the tracing backend an unbounded set of operations. The
			// path stays on the span as url.path.
			route := c.FullPath()
			spanName := c.Request.Method
			if route != "" {
				spanName += " " + route
			}

			// Extract the upstream trace context before starting the server
			// span: the W3C headers, or the framework's own trace id header
			// when they carry none (see the propagators otel.Init installs).
			parentCtx := otel.GetTextMapPropagator().Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))

			// Start new span
			ctx, span = gstotel.StartSpan(parentCtx, spanName, trace.WithSpanKind(trace.SpanKindServer))
			ctx = gstotel.ContextWithRequestRootSpan(ctx)

			// Extract OTEL trace ID and span ID
			spanContext := span.SpanContext()
			if spanContext.HasTraceID() {
				traceID = spanContext.TraceID().String()
				spanID = spanContext.SpanID().String()
			}

			// Performance: submit all request-scoped attributes of one phase in
			// a single SetAttributes call. Every call on a recording span locks
			// the span, grows the attribute slice, and re-runs deduplication,
			// which dominated allocation profiles under load. When adding
			// attributes, extend the batches below instead of adding
			// SetAttributes calls, and keep the recording gate so the
			// sampled-out path stays allocation-free. The trace and span IDs
			// are deliberately not attributes: every span already carries both
			// in its own envelope, which is what tracing backends index and
			// display. Repeating them would cost two string formats, two
			// attributes and their encoding on every sampled request, for
			// values a backend already has.
			recording = gstotel.IsSpanRecording(span)
			if recording {
				// The scheme is the one the client used: TLS on this
				// connection or the one a proxy in front declares, the
				// answer a service reads as IsHTTPS.
				scheme := schemeHTTP
				if requestctx.GinTLS(c) {
					scheme = schemeHTTPS
				}
				host, port := splitHostPort(c.Request.Host)
				attrs := make([]attribute.KeyValue, 0, requestSpanAttrCap)
				attrs = append(
					attrs,
					semconv.HTTPRequestMethodKey.String(c.Request.Method),
					semconv.URLPath(c.Request.URL.Path),
					scheme,
					semconv.ServerAddress(host),
					semconv.ClientAddress(requestctx.GinClientIP(c)),
				)

				// What a request does not carry is left off the span, as the
				// conventions ask, rather than recorded empty.
				if port > 0 {
					attrs = append(attrs, semconv.ServerPort(port))
				}
				if query := c.Request.URL.RawQuery; query != "" {
					attrs = append(attrs, semconv.URLQuery(query))
				}
				if route != "" {
					attrs = append(attrs, semconv.HTTPRoute(route))
				}
				if userAgent := c.Request.UserAgent(); userAgent != "" {
					attrs = append(attrs, semconv.UserAgentOriginal(userAgent))
				}
				if c.Request.ContentLength > 0 {
					attrs = append(attrs, semconv.HTTPRequestBodySizeKey.Int64(c.Request.ContentLength))
				}
				span.SetAttributes(attrs...)

				// Record start time for duration calculation
				start = time.Now()
				c.Set("request_start_time", start)
			}

			// Store span in context for use in handlers
			c.Set("otel_span", span)

			// Defer span completion. Response attributes are batched into one
			// SetAttributes call for the same performance reason as the request
			// batch above; only the rare error path submits a second batch.
			defer func() {
				if recording {
					statusCode := c.Writer.Status()
					attrs := make([]attribute.KeyValue, 0, responseSpanAttrCap)
					attrs = append(
						attrs,
						semconv.HTTPResponseStatusCode(statusCode),
						attribute.Int64("http.duration_ms", time.Since(start).Milliseconds()),
					)
					if size := c.Writer.Size(); size > 0 {
						attrs = append(attrs, semconv.HTTPResponseBodySize(size))
					}

					// The status follows the OpenTelemetry HTTP semantic
					// conventions, which otelgrpc holds the gRPC line's server
					// spans to as well: a server error sets Error, a client
					// error leaves the status Unset, as does success. Ok is
					// never set, the SDK treating it as final, so a request the
					// handler reported errors on turns Error below whatever its
					// status code.
					if statusCode >= http.StatusInternalServerError {
						span.SetStatus(codes.Error, "HTTP "+strconv.Itoa(statusCode))
						attrs = append(attrs, attribute.Bool("error", true))
					}
					span.SetAttributes(attrs...)

					// Record any errors from the request context
					if len(c.Errors) > 0 {
						errAttrs := make([]attribute.KeyValue, 0, len(c.Errors)+1)
						errAttrs = append(errAttrs, attribute.Bool("error", true))
						span.SetStatus(codes.Error, c.Errors.String())
						for i, err := range c.Errors {
							errAttrs = append(errAttrs, attribute.String("error."+strconv.Itoa(i), err.Error()))
						}
						span.SetAttributes(errAttrs...)
					}
				}

				span.End()
			}()
		} else {
			// Tracing off: the id the request carries when it is a W3C trace
			// id, a new one otherwise, the rule the propagator applies with
			// tracing on (see requestctx.TraceIDOf); the span id is new either
			// way.
			ctx = c.Request.Context()
			traceID = requestctx.TraceIDOf(c.Request.Header.Get(execctx.HeaderTraceID))
			spanID = util.SpanID()
		}

		// The trace id is the identity of this execution: everything downstream
		// that annotates its output — statement comments, the SQL log, the
		// business log — reads it off the request context, whether it runs on a
		// service context or on the raw request context. The gin keys below
		// serve the parts that only see gin.
		c.Request = c.Request.WithContext(execctx.WithTraceID(ctx, traceID))

		// Set trace fields in gin context.
		c.Set(execctx.KeyTraceID, traceID)
		c.Set(execctx.KeySpanID, spanID)

		// Set X-Trace-ID header for callers.
		c.Header(execctx.HeaderTraceID, traceID)

		// Process request. The request duration is part of the response batch
		// above, so no SetAttributes call is made here.
		c.Next()
	}
}

// splitHostPort splits the Host a request names its server by into the
// address and the port the span carries apart (server.address, server.port):
// "sample.test:8080" into "sample.test" and 8080, "[2001:db8::1]:8080" into
// "2001:db8::1" and 8080. A Host naming no port, "sample.test" or
// "[2001:db8::1]", gives port 0, as does one whose port is no port number.
func splitHostPort(hostport string) (host string, port int) {
	// No colon after the address, bracketed or not: the Host names no port,
	// which net.SplitHostPort would answer with an error allocated on every
	// such request.
	if strings.LastIndexByte(hostport, ':') <= strings.LastIndexByte(hostport, ']') {
		return strings.TrimSuffix(strings.TrimPrefix(hostport, "["), "]"), 0
	}
	host, portText, err := net.SplitHostPort(hostport)
	if err != nil {
		return hostport, 0
	}
	parsed, err := strconv.ParseUint(portText, 10, 16)
	if err != nil {
		return host, 0
	}
	return host, int(parsed)
}

// requestSpan returns the server span tracing opened for the request or,
// when there is none, the span current in the request context.
func requestSpan(c *gin.Context) trace.Span {
	if span, exists := c.Get("otel_span"); exists {
		if otelSpan, ok := span.(trace.Span); ok {
			return otelSpan
		}
	}
	return trace.SpanFromContext(c.Request.Context())
}
