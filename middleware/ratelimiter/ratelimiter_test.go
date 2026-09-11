package ratelimiter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/types"
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

// TestRateLimiterInstancesKeepSeparateBuckets pins that a bucket belongs to one
// limiter as well as to one key. Two limiters chained on a route both key by
// client IP by default — a generous one in front of the whole API and a strict
// one for a sensitive path, say — and each must spend its own burst under its
// own parameters instead of sharing whichever bucket the first one created.
func TestRateLimiterInstancesKeepSeparateBuckets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	slowRefill := WithRate(rate.Limit(0.001))
	engine := gin.New()
	engine.GET("/probe",
		RateLimiter(slowRefill, WithBurst(3)),
		RateLimiter(slowRefill, WithBurst(2)),
		func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	const client = "198.51.100.21:5000"
	require.Equal(t, http.StatusOK, probe(engine, client).Code)
	require.Equal(t, http.StatusOK, probe(engine, client).Code,
		"the strict limiter still holds the second of its own two tokens")
	require.Equal(t, http.StatusTooManyRequests, probe(engine, client).Code,
		"the strict limiter has spent its own burst")
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

// TestRateLimiterRefusesWhenCacheFails pins the failure mode: a limiter whose
// cache cannot answer refuses the request instead of letting it through, and
// reports the fault as the server's rather than the client's.
func TestRateLimiterRefusesWhenCacheFails(t *testing.T) {
	original := limiterCache
	limiterCache = func() types.Cache[*rate.Limiter] { return failingLimiterCache{} }
	t.Cleanup(func() { limiterCache = original })

	engine := newLimitedEngine(t, WithKeyFunc(constantKey(t.Name())))

	refused := probe(engine, "")
	require.Equal(t, http.StatusInternalServerError, refused.Code)
	require.Contains(t, refused.Body.String(), "rate limiter unavailable")
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

// constantKey returns a KeyFunc pinning every request to one bucket, whatever
// address it arrives from.
func constantKey(key string) func(*gin.Context) string {
	return func(*gin.Context) string { return key }
}

var errLimiterCacheDown = errors.New("limiter cache down")

// failingLimiterCache is a limiter cache whose every operation fails with an
// error other than a miss.
type failingLimiterCache struct{}

func (failingLimiterCache) Get(context.Context, string) (*rate.Limiter, error) {
	return nil, errLimiterCacheDown
}

func (failingLimiterCache) Set(context.Context, string, *rate.Limiter, time.Duration) error {
	return errLimiterCacheDown
}

func (failingLimiterCache) Delete(context.Context, string) error {
	return errLimiterCacheDown
}

func (failingLimiterCache) Exists(context.Context, string) bool {
	return false
}
