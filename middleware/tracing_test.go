package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger"
	pkgzap "github.com/hydroan/gst/logger/zap"
	gstotel "github.com/hydroan/gst/provider/otel"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestTracingUsesIncomingTraceparent(t *testing.T) {
	setupTracingTest(t)

	const incomingTraceID = "11111111111111111111111111111111"

	router := gin.New()
	router.Use(Tracing())
	router.GET("/api/ping", func(c *gin.Context) {
		spanContext := oteltrace.SpanFromContext(c.Request.Context()).SpanContext()
		require.True(t, spanContext.HasTraceID())
		require.Equal(t, incomingTraceID, spanContext.TraceID().String())
		require.Equal(t, incomingTraceID, c.GetString(consts.TRACE_ID))
		require.Equal(t, incomingTraceID, c.GetString(consts.REQUEST_ID))
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
	router.Use(Tracing())
	router.GET("/api/ping", func(c *gin.Context) {
		spanContext := oteltrace.SpanFromContext(c.Request.Context()).SpanContext()
		require.True(t, spanContext.HasTraceID())
		require.Equal(t, incomingTraceID, spanContext.TraceID().String())
		require.Equal(t, incomingTraceID, c.GetString(consts.TRACE_ID))
		require.Equal(t, incomingTraceID, c.GetString(consts.REQUEST_ID))
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
	setupTracingTestWithSampler(t, config.TracesSamplerAlwaysOff)

	router := gin.New()
	router.Use(Tracing())
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

func TestMiddlewareWrapperKeepsMiddlewareSpanWhenSamplerDrops(t *testing.T) {
	setupTracingTestWithSampler(t, config.TracesSamplerAlwaysOff)

	var rootSpanContext oteltrace.SpanContext
	var middlewareSpanContext oteltrace.SpanContext

	router := gin.New()
	router.Use(Tracing())
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

func TestTracingMarksHTTPSpanAsRequestRoot(t *testing.T) {
	source := readMiddlewareSource(t, "tracing.go")
	require.Contains(t, source, "ctx = gstotel.ContextWithRequestRootSpan(ctx)")
}

func TestMiddlewareWrapperStartsMiddlewareSpansFromRequestRoot(t *testing.T) {
	source := readMiddlewareSource(t, "wrapper.go")
	require.Contains(t, source, "parentCtx := gstotel.RequestRootContext(originalCtx)")
}

func setupTracingTest(t *testing.T) {
	t.Helper()

	setupTracingTestWithEndpoint(t, "http://127.0.0.1:1/v1/traces")
}

func setupTracingTestWithEndpoint(t *testing.T, endpoint string) {
	t.Helper()
	setupTracingTestWithEndpointAndSampler(t, endpoint, config.TracesSamplerParentBasedAlwaysOn)
}

func setupTracingTestWithSampler(t *testing.T, sampler config.TracesSampler) {
	t.Helper()

	setupTracingTestWithEndpointAndSampler(t, "http://127.0.0.1:1/v1/traces", sampler)
}

func setupTracingTestWithEndpointAndSampler(t *testing.T, endpoint string, sampler config.TracesSampler) {
	t.Helper()

	gin.SetMode(gin.TestMode)

	originalConfig := config.App
	config.App = new(config.Config)
	config.App.OTEL.Enable = true
	config.App.OTEL.ServiceName = "gst-test"
	config.App.OTEL.ExporterOTLPProtocol = config.OTLPProtocolHTTPProtobuf
	config.App.OTEL.ExporterOTLPTracesEndpoint = endpoint
	config.App.OTEL.ExporterOTLPCompression = config.OTLPCompressionNone
	config.App.OTEL.TracesSampler = sampler
	config.App.OTEL.BSPMaxQueueSize = 100
	config.App.OTEL.BSPMaxExportBatchSize = 100
	config.App.OTEL.BSPScheduleDelay = 10 * time.Millisecond
	config.App.OTEL.BSPExportTimeout = time.Second
	t.Cleanup(func() {
		config.App = originalConfig
	})

	originalOTELLogger := logger.OTEL
	logger.OTEL = pkgzap.New("/dev/null")
	t.Cleanup(func() {
		logger.OTEL = originalOTELLogger
	})

	gstotel.Close()
	require.NoError(t, gstotel.Init())
	t.Cleanup(func() {
		gstotel.Close()
	})
}

func readMiddlewareSource(t *testing.T, filename string) string {
	t.Helper()

	source, err := os.ReadFile(filename)
	require.NoError(t, err)
	return string(source)
}
