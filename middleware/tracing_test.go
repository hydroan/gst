package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/testutil/oteltest"
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
		require.Equal(t, incomingTraceID, execctx.FromContext(c.Request.Context()).TraceID)
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
		require.Equal(t, incomingTraceID, execctx.FromContext(c.Request.Context()).TraceID)
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	req.Header.Set(consts.HEADER_TRACE_ID, incomingTraceID)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.Equal(t, incomingTraceID, w.Header().Get(consts.HEADER_TRACE_ID))
}

// TestTracingStampsGeneratedTraceIDWhenOTELDisabled pins the fallback branch:
// with tracing off the middleware still stamps the id it publishes, so the
// annotations downstream carry the same id the caller was handed.
func TestTracingStampsGeneratedTraceIDWhenOTELDisabled(t *testing.T) {
	original := config.App.OTEL.Enabled
	config.App.OTEL.Enabled = false
	t.Cleanup(func() { config.App.OTEL.Enabled = original })

	var stamped, published string
	router := gin.New()
	router.Use(tracing())
	router.GET("/api/ping", func(c *gin.Context) {
		stamped = execctx.FromContext(c.Request.Context()).TraceID
		published = c.GetString(consts.TRACE_ID)
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/ping", nil))

	require.NotEmpty(t, stamped)
	require.Equal(t, published, stamped)
	require.Equal(t, stamped, w.Header().Get(consts.HEADER_TRACE_ID))
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

// TestTracingSpanAttributesFitTheCapacityInTheWorstCase pins the two batches
// the root span carries — a request with a content type and length, a failed
// response — plus one attribute per error the handler reported, so an
// attribute added without bumping its capacity fails here instead of
// regrowing the slice on every traced request.
func TestTracingSpanAttributesFitTheCapacityInTheWorstCase(t *testing.T) {
	setupTracingTest(t)
	recorder := oteltest.Record(t)

	router := gin.New()
	router.Use(tracing())
	router.POST("/api/samples", func(c *gin.Context) {
		_ = c.Error(errors.New("first sample failure"))
		_ = c.Error(errors.New("second sample failure"))
		c.Status(http.StatusInternalServerError)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/samples", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Length", "2")
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError, w.Code)

	span := oteltest.EndedNamed(t, recorder, "POST /api/samples")
	// The error batch adds one attribute per error and re-sets the error flag
	// the response batch already carries; a span keeps one value per key.
	const errorsReported = 2
	require.Len(t, span.Attributes(), requestSpanAttrCap+responseSpanAttrCap+errorsReported,
		"the worst case must fill both batches exactly: a new attribute bumps requestSpanAttrCap or responseSpanAttrCap")
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
