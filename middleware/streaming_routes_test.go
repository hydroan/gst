package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/config"
	"github.com/sony/gobreaker"
	"github.com/stretchr/testify/require"
)

func TestStreamingRouteRegistry(t *testing.T) {
	MarkStreamingRoute("get", "/api/registry/events")

	require.True(t, IsStreamingRoute("GET", "/api/registry/events"))
	require.True(t, IsStreamingRoute("get", "/api/registry/events"))
	require.False(t, IsStreamingRoute("POST", "/api/registry/events"))
	require.False(t, IsStreamingRoute("GET", "/api/registry/other"))
}

func TestBodyLoggerLeavesStreamingResponseUnwrapped(t *testing.T) {
	setupHTTPBodyLoggerTest(t, config.HTTPBodyLogger{
		Enabled:     true,
		LogRequest:  config.HTTPBodyLogModeAll,
		LogResponse: config.HTTPBodyLogModeAll,
		MaxBodySize: "64KB",
	})
	MarkStreamingRoute(http.MethodGet, "/api/bodylogger/stream")

	router := gin.New()
	router.Use(BodyLogger())
	var streamWrapped, plainWrapped bool
	router.GET("/api/bodylogger/stream", func(c *gin.Context) {
		_, streamWrapped = c.Writer.(*bodyLogWriter)
		c.Status(http.StatusOK)
	})
	router.GET("/api/bodylogger/plain", func(c *gin.Context) {
		_, plainWrapped = c.Writer.(*bodyLogWriter)
		c.Status(http.StatusOK)
	})

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/bodylogger/stream", nil))
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/bodylogger/plain", nil))

	require.False(t, streamWrapped, "a streaming response must not be teed into the body log buffer")
	require.True(t, plainWrapped, "a regular response must still be captured")
}

func TestCircuitBreakerPassesStreamingRouteThrough(t *testing.T) {
	originalBreaker := cb
	t.Cleanup(func() { cb = originalBreaker })

	// A breaker that opens on the first failure and stays open for the whole
	// test: regular requests are refused, so a streaming request only gets
	// through when the middleware skips the breaker entirely.
	cb = gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "streaming-test",
		Timeout:     time.Hour,
		ReadyToTrip: func(gobreaker.Counts) bool { return true },
	})
	_, err := cb.Execute(func() (any, error) { return nil, errors.New("trip the breaker") })
	require.Error(t, err)
	require.Equal(t, gobreaker.StateOpen, cb.State())

	MarkStreamingRoute(http.MethodGet, "/api/breaker/stream")

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CircuitBreaker())
	handle := func(c *gin.Context) { c.String(http.StatusOK, "ok") }
	router.GET("/api/breaker/stream", handle)
	router.GET("/api/breaker/plain", handle)

	stream := httptest.NewRecorder()
	router.ServeHTTP(stream, httptest.NewRequest(http.MethodGet, "/api/breaker/stream", nil))
	require.Equal(t, http.StatusOK, stream.Code, "a streaming request must bypass the open breaker")

	plain := httptest.NewRecorder()
	router.ServeHTTP(plain, httptest.NewRequest(http.MethodGet, "/api/breaker/plain", nil))
	require.Equal(t, http.StatusServiceUnavailable, plain.Code, "a regular request must still hit the breaker")
}

func TestTimeoutLeavesStreamingRouteAlone(t *testing.T) {
	MarkStreamingRoute(http.MethodGet, "/api/timeout/stream")

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Timeout(30 * time.Millisecond))
	router.GET("/api/timeout/stream", func(c *gin.Context) {
		// Outlives the timeout on purpose; only the streaming exemption lets
		// this response finish.
		time.Sleep(100 * time.Millisecond)
		c.String(http.StatusOK, "finished")
	})

	stream := httptest.NewRecorder()
	router.ServeHTTP(stream, httptest.NewRequest(http.MethodGet, "/api/timeout/stream", nil))
	require.Equal(t, http.StatusOK, stream.Code)
	require.Equal(t, "finished", stream.Body.String())

	// The timed-out counterpart is deliberately not exercised: the timeout
	// branch hands the gin context back to the engine while the handler
	// goroutine still runs, an inherent data race in Timeout that predates
	// the streaming registry and trips the race detector on any request that
	// actually times out. Cover it when that defect is fixed.
}
