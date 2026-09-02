package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/middleware/ratelimiter"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// wrapperStampKey keys the value a test middleware attaches to the request
// context, standing in for the session or tenant a real middleware attaches.
type wrapperStampKey struct{}

// TestMiddlewareWrapperKeepsContextOfMiddlewareThatReturns pins the contract a
// middleware that only acts before the handler relies on: it attaches values
// to the request context and returns without calling c.Next(), and the
// handler still sees those values while running under the request root span
// rather than under the finished middleware span.
func TestMiddlewareWrapperKeepsContextOfMiddlewareThatReturns(t *testing.T) {
	setupTracingTest(t)

	var rootSpanID, handlerSpanID oteltrace.SpanID
	var handlerStamp any

	router := gin.New()
	router.Use(tracing())
	router.Use(func(c *gin.Context) {
		rootSpanID = oteltrace.SpanFromContext(c.Request.Context()).SpanContext().SpanID()
	})
	router.Use(middlewareWrapper("stamp", func(c *gin.Context) {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), wrapperStampKey{}, "stamped"))
	}))
	router.GET("/api/ping", func(c *gin.Context) {
		handlerStamp = c.Request.Context().Value(wrapperStampKey{})
		handlerSpanID = oteltrace.SpanFromContext(c.Request.Context()).SpanContext().SpanID()
		c.Status(http.StatusNoContent)
	})

	w := performWrapperRequest(router)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.Equal(t, "stamped", handlerStamp, "values the middleware attached must reach the handler")
	require.Equal(t, rootSpanID, handlerSpanID, "the handler must run under the request root, not under the finished middleware span")
}

// TestMiddlewareWrapperEndsSpanBeforeHandlerWhenMiddlewareReturns verifies that
// a middleware which returns without calling c.Next() has its span closed by
// the time the handler runs, so the span measures the middleware alone. The
// rate limiter is the framework middleware of that shape.
func TestMiddlewareWrapperEndsSpanBeforeHandlerWhenMiddlewareReturns(t *testing.T) {
	setupTracingTest(t)
	recorder := recordSpans(t)

	var endedBeforeHandler []string

	router := gin.New()
	router.Use(tracing())
	router.Use(middlewareWrapper("RateLimiter", ratelimiter.RateLimiter()))
	router.GET("/api/ping", func(c *gin.Context) {
		endedBeforeHandler = endedSpanNames(recorder)
		c.Status(http.StatusNoContent)
	})

	w := performWrapperRequest(router)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.Contains(t, endedBeforeHandler, "middleware.RateLimiter")
}

// TestMiddlewareWrapperCoversDownstreamOfMiddlewareThatCallsNext verifies the
// other middleware shape: one that calls c.Next() to observe the response
// keeps its span open across the handler and still hands its context values
// down.
func TestMiddlewareWrapperCoversDownstreamOfMiddlewareThatCallsNext(t *testing.T) {
	setupTracingTest(t)
	recorder := recordSpans(t)

	var endedBeforeHandler []string
	var handlerStamp any

	router := gin.New()
	router.Use(tracing())
	router.Use(middlewareWrapper("observe", func(c *gin.Context) {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), wrapperStampKey{}, "stamped"))
		c.Next()
	}))
	router.GET("/api/ping", func(c *gin.Context) {
		endedBeforeHandler = endedSpanNames(recorder)
		handlerStamp = c.Request.Context().Value(wrapperStampKey{})
		c.Status(http.StatusNoContent)
	})

	w := performWrapperRequest(router)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.Equal(t, "stamped", handlerStamp)
	require.False(t, slices.Contains(endedBeforeHandler, "middleware.Observe"), "a middleware that calls Next keeps its span open over the handler")
	require.Contains(t, endedSpanNames(recorder), "middleware.Observe")
}

func TestMiddlewareWrapperKeepsMiddlewareSpanWhenSamplerDrops(t *testing.T) {
	setupTracingTestWithSampler(t, config.TracesSamplerAlwaysOff)

	var rootSpanContext oteltrace.SpanContext
	var middlewareSpanContext oteltrace.SpanContext

	router := gin.New()
	router.Use(tracing())
	router.Use(middlewareWrapper("test", func(c *gin.Context) {
		rootSpan, exists := c.Get("otel_span")
		require.True(t, exists)

		root, ok := rootSpan.(oteltrace.Span)
		require.True(t, ok)
		rootSpanContext = root.SpanContext()

		currentSpan := oteltrace.SpanFromContext(c.Request.Context())
		middlewareSpanContext = currentSpan.SpanContext()
		require.False(t, currentSpan.IsRecording())
	}))
	router.GET("/api/ping", func(c *gin.Context) {
		currentSpanContext := oteltrace.SpanFromContext(c.Request.Context()).SpanContext()
		require.Equal(t, rootSpanContext.SpanID(), currentSpanContext.SpanID())
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.True(t, rootSpanContext.HasTraceID())
	require.True(t, middlewareSpanContext.HasTraceID())
	require.Equal(t, rootSpanContext.TraceID(), middlewareSpanContext.TraceID())
	require.NotEqual(t, rootSpanContext.SpanID(), middlewareSpanContext.SpanID())
}

func TestMiddlewareWrapperStartsMiddlewareSpansFromRequestRoot(t *testing.T) {
	source := readMiddlewareSource(t, "wrapper.go")
	require.Contains(t, source, "parentCtx := gstotel.RequestRootContext(originalCtx)")
}

func performWrapperRequest(router *gin.Engine) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/ping", nil))
	return w
}

// recordSpans attaches an in-memory span recorder to the SDK tracer provider
// that setupTracingTest installed, so a test can read back which spans have
// ended at any point of a request.
func recordSpans(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	provider, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider)
	require.True(t, ok, "otel.Init must install the SDK tracer provider globally")
	recorder := tracetest.NewSpanRecorder()
	provider.RegisterSpanProcessor(recorder)
	return recorder
}

// endedSpanNames returns the names of every span the recorder has seen end.
func endedSpanNames(recorder *tracetest.SpanRecorder) []string {
	ended := recorder.Ended()
	names := make([]string, 0, len(ended))
	for _, span := range ended {
		names = append(names, span.Name())
	}
	return names
}
