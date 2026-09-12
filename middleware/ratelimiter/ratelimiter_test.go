package ratelimiter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

// slowRefill is far longer than any test run, so a token a test spends does not
// come back while the test is still looking.
const slowRefill = time.Hour

// TestRateLimiterAllowsBurstThenRejects pins the core contract: a key may spend
// its whole bucket, and the request after that is refused with 429.
func TestRateLimiterAllowsBurstThenRejects(t *testing.T) {
	engine := newLimitedEngine(t, WithBucket(2, slowRefill), WithKeyFunc(constantKey(t.Name())))

	require.Equal(t, http.StatusOK, probe(engine, "").Code)
	require.Equal(t, http.StatusOK, probe(engine, "").Code)

	refused := probe(engine, "")
	require.Equal(t, http.StatusTooManyRequests, refused.Code)
	require.Contains(t, refused.Body.String(), "too many requests")
}

// TestRateLimiterRefillsOneTokenPerInterval pins the refill half of the bucket:
// a spent token comes back once one refill interval has passed.
func TestRateLimiterRefillsOneTokenPerInterval(t *testing.T) {
	engine := newLimitedEngine(t, WithBucket(1, 50*time.Millisecond), WithKeyFunc(constantKey(t.Name())))

	require.Equal(t, http.StatusOK, probe(engine, "").Code)
	require.Equal(t, http.StatusTooManyRequests, probe(engine, "").Code)

	time.Sleep(100 * time.Millisecond)
	require.Equal(t, http.StatusOK, probe(engine, "").Code)
}

// TestRateLimiterIsolatesKeys pins that a bucket belongs to one key: exhausting
// one key leaves another free to spend its own.
func TestRateLimiterIsolatesKeys(t *testing.T) {
	var key string
	engine := newLimitedEngine(t, WithBucket(1, slowRefill), WithKeyFunc(func(*gin.Context) string { return key }))

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
	engine := gin.New()
	engine.GET("/probe",
		RateLimiter(WithBucket(3, slowRefill)),
		RateLimiter(WithBucket(2, slowRefill)),
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
	engine := newLimitedEngine(t, WithBucket(1, slowRefill))

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
		WithBucket(1, slowRefill),
		WithKeyFunc(constantKey(t.Name())),
		WithSkipFunc(func(c *gin.Context) bool { return c.GetHeader("X-Probe-Skip") == "yes" }),
	)

	require.Equal(t, http.StatusOK, probe(engine, "").Code)
	require.Equal(t, http.StatusTooManyRequests, probe(engine, "").Code)
	require.Equal(t, http.StatusOK, probe(engine, "", "X-Probe-Skip", "yes").Code)
}

// TestRateLimiterRefusesWhenCacheFails pins the failure mode: a limiter whose
// cache cannot answer refuses the request instead of letting it through, and
// reports the fault as the server's rather than the client's.
func TestRateLimiterRefusesWhenCacheFails(t *testing.T) {
	useLimiterCache(t, failingLimiterCache{})
	engine := newLimitedEngine(t, WithKeyFunc(constantKey(t.Name())))

	refused := probe(engine, "")
	require.Equal(t, http.StatusInternalServerError, refused.Code)
	require.Contains(t, refused.Body.String(), "rate limiter unavailable")
}

// TestRateLimiterRestartsBucketLifetimeOnEveryRequest pins how a bucket expires:
// it is kept for capacity × refill interval of idle time, long enough to refill
// completely so that dropping it loses nothing, and every request restarts that
// time, the refused one included. A lifetime counted from creation would hand a
// client hammering an empty bucket a full one each time it ran out.
func TestRateLimiterRestartsBucketLifetimeOnEveryRequest(t *testing.T) {
	store := &recordingLimiterCache{limiters: make(map[string]*rate.Limiter)}
	useLimiterCache(t, store)
	engine := newLimitedEngine(t, WithBucket(2, time.Minute), WithKeyFunc(constantKey(t.Name())))

	require.Equal(t, http.StatusOK, probe(engine, "").Code)
	require.Equal(t, http.StatusOK, probe(engine, "").Code)
	require.Equal(t, http.StatusTooManyRequests, probe(engine, "").Code)

	// The extra millisecond covers the cache keeping expiry to the millisecond.
	lifetime := 2*time.Minute + time.Millisecond
	require.Equal(t, []time.Duration{lifetime, lifetime, lifetime}, store.lifetimes)
}

// newLimitedEngine builds a router whose only route is rate limited by a
// limiter built from opts.
func newLimitedEngine(t *testing.T, opts ...Option) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.GET("/probe", RateLimiter(opts...), func(c *gin.Context) { c.String(http.StatusOK, "ok") })
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

// useLimiterCache puts cache behind every limiter for the rest of the test.
func useLimiterCache(t *testing.T, cache types.Cache[*rate.Limiter]) {
	t.Helper()

	original := limiterCache
	limiterCache = func() types.Cache[*rate.Limiter] { return cache }
	t.Cleanup(func() { limiterCache = original })
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

// recordingLimiterCache is a limiter cache held in a map that records the
// lifetime of every write.
type recordingLimiterCache struct {
	limiters  map[string]*rate.Limiter
	lifetimes []time.Duration
}

func (c *recordingLimiterCache) Get(_ context.Context, key string) (*rate.Limiter, error) {
	limiter, ok := c.limiters[key]
	if !ok {
		return nil, types.ErrEntryNotFound
	}
	return limiter, nil
}

func (c *recordingLimiterCache) Set(_ context.Context, key string, limiter *rate.Limiter, ttl time.Duration) error {
	c.limiters[key] = limiter
	c.lifetimes = append(c.lifetimes, ttl)
	return nil
}

func (c *recordingLimiterCache) Delete(_ context.Context, key string) error {
	delete(c.limiters, key)
	return nil
}

func (c *recordingLimiterCache) Exists(_ context.Context, key string) bool {
	_, ok := c.limiters[key]
	return ok
}
