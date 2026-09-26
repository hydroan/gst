package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/logger"
	prommetrics "github.com/hydroan/gst/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// TestAccessLoggerFieldsFitTheCapacityInTheWorstCase pins accessLogFieldCap
// to the entry of a request a recording span traces — the span id, the one
// optional field, present — so a field added to accessLogger without bumping
// the capacity fails here instead of regrowing the slice on every request.
func TestAccessLoggerFieldsFitTheCapacityInTheWorstCase(t *testing.T) {
	setupTracingTest(t)

	core, logs := observer.New(zapcore.InfoLevel)
	originalLogger := logger.Gin
	logger.Gin = zap.New(core)
	t.Cleanup(func() { logger.Gin = originalLogger })

	// The request metrics are stood in for rather than built by
	// prommetrics.Init, which registers with the default registry and so
	// cannot run twice.
	labels := []string{"method", "path", "status"}
	originalTotal, originalDuration := prommetrics.HTTPRequestsTotal, prommetrics.HTTPRequestDuration
	prommetrics.HTTPRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "http_requests_probe"}, labels)
	prommetrics.HTTPRequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "http_request_duration_probe"}, labels)
	t.Cleanup(func() {
		prommetrics.HTTPRequestsTotal, prommetrics.HTTPRequestDuration = originalTotal, originalDuration
	})

	router := gin.New()
	router.Use(tracing(), accessLogger())
	router.GET("/api/ping", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/ping?page=1", nil))
	require.Equal(t, http.StatusNoContent, w.Code)

	entries := logs.All()
	require.Len(t, entries, 1)
	require.Len(t, entries[0].Context, accessLogFieldCap,
		"the worst case must fill the capacity exactly: a new field bumps accessLogFieldCap, a dropped one lowers it")
	require.Contains(t, entries[0].ContextMap(), "span_id", "the worst case must carry the optional span id")
}
