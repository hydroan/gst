package ratelimiter

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

// TestRateLimiterAllowsBurstThenRejects pins the core contract: a key may spend
// its whole bucket, and the request after that is refused with 429.
func TestRateLimiterAllowsBurstThenRejects(t *testing.T) {
	engine := newLimitedEngine(t, WithBurst(2), WithKeyFunc(constantKey(t.Name())))

	require.Equal(t, http.StatusOK, probe(engine, "").Code)
	require.Equal(t, http.StatusOK, probe(engine, "").Code)

	refused := probe(engine, "")
	require.Equal(t, http.StatusTooManyRequests, refused.Code)
	require.Contains(t, refused.Body.String(), "too many requests")
}

// TestRateLimiterIsolatesKeys pins that a bucket belongs to one key: exhausting
// one key leaves another free to spend its own.
func TestRateLimiterIsolatesKeys(t *testing.T) {
	var key string
	engine := newLimitedEngine(t, WithBurst(1), WithKeyFunc(func(*gin.Context) string { return key }))

	key = t.Name() + "/first"
	require.Equal(t, http.StatusOK, probe(engine, "").Code)
	require.Equal(t, http.StatusTooManyRequests, probe(engine, "").Code)

	key = t.Name() + "/second"
	require.Equal(t, http.StatusOK, probe(engine, "").Code)
}

// TestRateLimiterKeysByClientIPByDefault pins the default key: without a
// KeyFunc each client address gets its own bucket.
func TestRateLimiterKeysByClientIPByDefault(t *testing.T) {
	engine := newLimitedEngine(t, WithBurst(1))

	require.Equal(t, http.StatusOK, probe(engine, "198.51.100.11:5000").Code)
	require.Equal(t, http.StatusTooManyRequests, probe(engine, "198.51.100.11:5001").Code,
		"a second port is the same client, so it shares the bucket")
	require.Equal(t, http.StatusOK, probe(engine, "198.51.100.12:5000").Code,
		"a different address must get its own bucket")
}

// TestRateLimiterHonorsSkipFunc pins the bypass: a skipped request never
// touches a bucket, so it stays allowed past the burst.
func TestRateLimiterHonorsSkipFunc(t *testing.T) {
	engine := newLimitedEngine(t,
		WithBurst(1),
		WithKeyFunc(constantKey(t.Name())),
		WithSkipFunc(func(c *gin.Context) bool { return c.GetHeader("X-Probe-Skip") == "yes" }),
	)

	require.Equal(t, http.StatusOK, probe(engine, "").Code)
	require.Equal(t, http.StatusTooManyRequests, probe(engine, "").Code)
	require.Equal(t, http.StatusOK, probe(engine, "", "X-Probe-Skip", "yes").Code)
}

// TestRateLimiterHonorsCustomLimitHandler pins that a custom handler owns the
// refusal response: it replaces the default 429 body and status.
func TestRateLimiterHonorsCustomLimitHandler(t *testing.T) {
	engine := newLimitedEngine(t,
		WithBurst(1),
		WithKeyFunc(constantKey(t.Name())),
		WithOnLimitReached(func(c *gin.Context) { c.String(http.StatusServiceUnavailable, "slow down") }),
	)

	require.Equal(t, http.StatusOK, probe(engine, "").Code)

	refused := probe(engine, "")
	require.Equal(t, http.StatusServiceUnavailable, refused.Code)
	require.Equal(t, "slow down", refused.Body.String())
}

// TestRateLimiterFallsBackToDefaultBurst pins that an option carrying an
// invalid value leaves the default in force rather than the value it named.
func TestRateLimiterFallsBackToDefaultBurst(t *testing.T) {
	engine := newLimitedEngine(t, WithBurst(-1), WithKeyFunc(constantKey(t.Name())))

	for i := range defaultBurst {
		require.Equal(t, http.StatusOK, probe(engine, "").Code, "request %d must fit the default burst", i+1)
	}
	require.Equal(t, http.StatusTooManyRequests, probe(engine, "").Code)
}

// newLimitedEngine builds a router whose only route is rate limited. The
// refill rate is deliberately far below one token per test run, so a bucket
// spent by one request is still empty for the next.
func newLimitedEngine(t *testing.T, opts ...Option) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.GET("/probe", RateLimiter(append([]Option{WithRate(rate.Limit(0.001))}, opts...)...),
		func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	return engine
}

// probe sends one request to the limited route. An empty remoteAddr keeps the
// address httptest assigns; headers are given as key/value pairs.
func probe(engine *gin.Engine, remoteAddr string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	return recorder
}

// constantKey returns a KeyFunc pinning every request to one bucket, which is
// how a test keeps its own limiter out of the package-wide cache shared with
// every other test.
func constantKey(key string) func(*gin.Context) string {
	return func(*gin.Context) string { return key }
}
