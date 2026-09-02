package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/testutil/oteltest"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestTracingUsesIncomingTraceparent(t *testing.T) {
	setupTracingTest(t)

	const incomingTraceID = "11111111111111111111111111111111"

	router := gin.New()
	router.Use(tracing())
	router.GET("/api/ping", func(c *gin.Context) {
		spanContext := oteltrace.SpanFromContext(c.Request.Context()).SpanContext()
		require.True(t, spanContext.HasTraceID())
		require.Equal(t, incomingTraceID, spanContext.TraceID().String())
		require.Equal(t, incomingTraceID, c.GetString(consts.TRACE_ID))
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	req.Header.Set("Traceparent", "00-"+incomingTraceID+"-2222222222222222-01")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.Equal(t, incomingTraceID, w.Header().Get(consts.HEADER_TRACE_ID))
}

func TestTracingUsesIncomingTraceIDHeader(t *testing.T) {
	setupTracingTest(t)

	const incomingTraceID = "33333333333333333333333333333333"

	router := gin.New()
	router.Use(tracing())
	router.GET("/api/ping", func(c *gin.Context) {
		spanContext := oteltrace.SpanFromContext(c.Request.Context()).SpanContext()
		require.True(t, spanContext.HasTraceID())
		require.Equal(t, incomingTraceID, spanContext.TraceID().String())
		require.Equal(t, incomingTraceID, c.GetString(consts.TRACE_ID))
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	req.Header.Set(consts.HEADER_TRACE_ID, incomingTraceID)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.Equal(t, incomingTraceID, w.Header().Get(consts.HEADER_TRACE_ID))
}

func TestTracingSkipsRecordingOnlyStateWhenSamplerDrops(t *testing.T) {
	setupTracingTest(t, oteltest.WithSampler(config.TracesSamplerAlwaysOff))

	router := gin.New()
	router.Use(tracing())
	router.GET("/api/ping", func(c *gin.Context) {
		span := oteltrace.SpanFromContext(c.Request.Context())
		require.True(t, span.SpanContext().HasTraceID())
		require.False(t, span.IsRecording())

		_, exists := c.Get("request_start_time")
		require.False(t, exists)
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.NotEmpty(t, w.Header().Get(consts.HEADER_TRACE_ID))
}

func TestTracingMarksHTTPSpanAsRequestRoot(t *testing.T) {
	source := readMiddlewareSource(t, "tracing.go")
	require.Contains(t, source, "ctx = gstotel.ContextWithRequestRootSpan(ctx)")
}

// setupTracingTest enables real tracing for one middleware test and puts gin
// into test mode; opts adjust the sampler or the exporter endpoint.
func setupTracingTest(t *testing.T, opts ...oteltest.Option) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	oteltest.Enable(t, opts...)
}

func readMiddlewareSource(t *testing.T, filename string) string {
	t.Helper()

	source, err := os.ReadFile(filename)
	require.NoError(t, err)
	return string(source)
}
