package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/middleware"
	"github.com/stretchr/testify/require"
)

// delayUnderTest is short enough to keep the tests fast and long enough for
// a missing delay to show against the time a request takes.
const delayUnderTest = 20 * time.Millisecond

// TestDelayHoldsEveryRequestForTheDuration pins that Delay holds a request
// at least the duration given before the handler answers.
func TestDelayHoldsEveryRequestForTheDuration(t *testing.T) {
	elapsed, w := serveDelayed(middleware.Delay(delayUnderTest))

	require.Equal(t, http.StatusNoContent, w.Code)
	require.GreaterOrEqual(t, elapsed, delayUnderTest)
}

// TestDelayRandomHoldsARequestAtLeastTheMinimum pins the bounds DelayRandom
// keeps: a request is held at least the minimum, and a maximum below the
// minimum holds it for the minimum instead of failing.
func TestDelayRandomHoldsARequestAtLeastTheMinimum(t *testing.T) {
	for name, delay := range map[string]gin.HandlerFunc{
		"within_a_range":         middleware.DelayRandom(delayUnderTest, 2*delayUnderTest),
		"maximum_below_minimum":  middleware.DelayRandom(delayUnderTest, delayUnderTest/2),
		"minimum_equals_maximum": middleware.DelayRandom(delayUnderTest, delayUnderTest),
	} {
		t.Run(name, func(t *testing.T) {
			elapsed, w := serveDelayed(delay)

			require.Equal(t, http.StatusNoContent, w.Code)
			require.GreaterOrEqual(t, elapsed, delayUnderTest)
		})
	}
}

// TestDelayWithConfigHoldsARequestForWhatTheFunctionSays pins that
// DelayWithConfig asks its function about the request at hand and holds the
// request for the duration it returns, and not at all for a duration of
// zero or less.
func TestDelayWithConfigHoldsARequestForWhatTheFunctionSays(t *testing.T) {
	var asked string
	elapsed, w := serveDelayed(middleware.DelayWithConfig(func(c *gin.Context) time.Duration {
		asked = c.Request.URL.Path
		return delayUnderTest
	}))
	require.Equal(t, http.StatusNoContent, w.Code)
	require.Equal(t, "/api/slow", asked)
	require.GreaterOrEqual(t, elapsed, delayUnderTest)

	for _, duration := range []time.Duration{0, -time.Hour} {
		_, w := serveDelayed(middleware.DelayWithConfig(func(*gin.Context) time.Duration { return duration }))
		require.Equal(t, http.StatusNoContent, w.Code, duration)
	}
}

// serveDelayed answers a request to /api/slow through delay and returns how
// long the request took and the response.
func serveDelayed(delay gin.HandlerFunc) (time.Duration, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(delay)
	router.GET("/api/slow", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	w := httptest.NewRecorder()
	start := time.Now()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/slow", nil))
	return time.Since(start), w
}
