package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/logger"
	prommetrics "github.com/hydroan/gst/internal/metrics"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// TestAccessLoggerFieldsFitTheCapacityInTheWorstCase pins accessLogFieldCap
// to the entry of a request a recording span traces, whose WebSocket
// connection closed, that reported an error — the span id, the close code
// and the error, the three optional fields, present — so a field added to
// accessLogger without bumping the capacity fails here instead of regrowing
// the slice on every request.
func TestAccessLoggerFieldsFitTheCapacityInTheWorstCase(t *testing.T) {
	setupTracingTest(t)

	core, logs := observer.New(zapcore.InfoLevel)
	originalLogger := logger.HTTP
	logger.HTTP = zap.New(core)
	t.Cleanup(func() { logger.HTTP = originalLogger })

	standInRequestMetrics(t)

	router := gin.New()
	router.Use(tracing(), accessLogger())
	router.GET("/api/ping", func(c *gin.Context) {
		_ = c.Error(errors.New("probe failed"))
		c.Set(requestctx.KeyCloseCode, 1000)
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/ping?page=1", nil))
	require.Equal(t, http.StatusNoContent, w.Code)

	entries := logs.All()
	require.Len(t, entries, 1)
	require.Len(t, entries[0].Context, accessLogFieldCap,
		"the worst case must fill the capacity exactly: a new field bumps accessLogFieldCap, a dropped one lowers it")
	require.Contains(t, entries[0].ContextMap(), "span_id", "the worst case must carry the optional span id")
	require.EqualValues(t, 1000, entries[0].ContextMap()["close_code"], "the status the connection closed with travels in a field of its own")
	require.Equal(t, "probe failed", entries[0].ContextMap()["error"], "the error the request reported travels in a field of its own")
}

// TestAccessLoggerCarriesTheTenantTheRequestActsIn pins the tenant field: the
// entry carries the tenant the request context holds once the chain has run,
// under the key the context stores it by, and "" for a request that
// established none.
func TestAccessLoggerCarriesTheTenantTheRequestActsIn(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	originalLogger := logger.HTTP
	logger.HTTP = zap.New(core)
	t.Cleanup(func() { logger.HTTP = originalLogger })
	standInRequestMetrics(t)

	router := gin.New()
	router.Use(accessLogger())
	router.GET("/api/records", func(c *gin.Context) {
		if tenantID := c.GetHeader("X-Probe-Tenant"); tenantID != "" {
			c.Set(requestctx.KeyTenantID, tenantID)
		}
		c.Status(http.StatusNoContent)
	})

	scoped := httptest.NewRequest(http.MethodGet, "/api/records", nil)
	scoped.Header.Set("X-Probe-Tenant", "alpha")
	router.ServeHTTP(httptest.NewRecorder(), scoped)
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/records", nil))

	entries := logs.All()
	require.Len(t, entries, 2)
	require.Equal(t, "alpha", entries[0].ContextMap()[requestctx.KeyTenantID])
	require.Empty(t, entries[1].ContextMap()[requestctx.KeyTenantID], "a request that established no tenant logs the field empty")
}

// standInRequestMetrics replaces the request metrics for the test, rather
// than building them with prommetrics.Init, which registers with the default
// registry and so cannot run twice.
func standInRequestMetrics(t *testing.T) {
	t.Helper()
	labels := []string{"method", "path", "status"}
	originalTotal, originalDuration := prommetrics.HTTPRequestsTotal, prommetrics.HTTPRequestDuration
	prommetrics.HTTPRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "http_requests_probe"}, labels)
	prommetrics.HTTPRequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "http_request_duration_probe"}, labels)
	t.Cleanup(func() {
		prommetrics.HTTPRequestsTotal, prommetrics.HTTPRequestDuration = originalTotal, originalDuration
	})
}
