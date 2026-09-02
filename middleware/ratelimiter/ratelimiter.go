package ratelimiter

import (
	"net/http"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/cache/freelru"
	"github.com/hydroan/gst/response"
	"github.com/hydroan/gst/types"
	"golang.org/x/time/rate"
)

const (
	defaultRate  = rate.Limit(10) // default: allow 10 requests per second
	defaultBurst = 50             // default token bucket capacity
	defaultTTL   = 24 * time.Hour // default expiration of an idle limiter
)

// limiterCache holds one limiter per key.
//
// It names a backend instead of going through the cache facade on purpose. A
// limiter is security state, and the facade's forwarded backend is a
// framework-wide choice that may change: an admission-weighted one would drop
// writes for keys it considers cold, and every request from such a key would
// then build a fresh full bucket and be let through. Pinning the backend
// keeps that decision from reaching this middleware. Eviction still hands a
// key a fresh bucket once the store is full, which is the ordinary cost of
// bounding a rate limiter's memory, but it takes real pressure rather than a
// policy judgement about one key.
//
// Resolution is deferred to the first request because the backend reads the
// cache configuration when it builds an instance, and at package init that
// configuration is not loaded yet.
var limiterCache = sync.OnceValue(func() types.Cache[*rate.Limiter] {
	return freelru.Cache[*rate.Limiter]()
})

// Config holds the configuration for the RateLimiter middleware.
type Config struct {
	// Rate is the number of requests allowed per second.
	// Defaults to 10 req/s if not set or non-positive.
	Rate rate.Limit

	// Burst is the maximum number of requests allowed to burst above the rate.
	// Defaults to 50 if not set or non-positive.
	Burst int

	// TTL is the duration after which an idle rate limiter is evicted from the cache.
	// Defaults to 24h if not set or non-positive.
	TTL time.Duration

	// KeyFunc extracts a unique key from the request to identify the rate limit subject.
	// Defaults to client IP if not set.
	//
	// Common examples:
	//   c.ClientIP()                              per client IP (default)
	//   c.GetString("user_id")                   per authenticated user
	//   c.FullPath()                              per route
	//   c.GetHeader("X-API-Key")                 per API key
	//   c.FullPath() + ":" + c.GetString("user_id")  per user per route
	KeyFunc func(*gin.Context) string

	// OnLimitReached is called when the rate limit is exceeded.
	// If set, it is responsible for writing the response; the default 429 response is skipped.
	// Defaults to a 429 JSON response if not set.
	OnLimitReached gin.HandlerFunc

	// SkipFunc determines whether rate limiting should be skipped for a request.
	// Returns true to bypass rate limiting (e.g. health checks, internal IPs).
	SkipFunc func(*gin.Context) bool
}

// RateLimiter returns a gin middleware that limits request rates per configurable key.
// Use functional options (WithRate, WithBurst, WithKeyFunc, etc.) to customize behavior.
//
// Example:
//
//	r.Use(ratelimiter.RateLimiter(
//	    ratelimiter.WithRate(rate.Every(100*time.Millisecond)),
//	    ratelimiter.WithBurst(20),
//	    ratelimiter.WithKeyFunc(func(c *gin.Context) string { return c.ClientIP() }),
//	    ratelimiter.WithSkipFunc(func(c *gin.Context) bool { return c.FullPath() == "/health" }),
//	))
func RateLimiter(opts ...Option) gin.HandlerFunc {
	conf := new(Config)
	for _, op := range opts {
		if op == nil {
			continue
		}
		op(conf)
	}
	if conf.Rate <= 0 {
		conf.Rate = defaultRate
	}
	if conf.Burst <= 0 {
		conf.Burst = defaultBurst
	}
	if conf.KeyFunc == nil {
		conf.KeyFunc = func(c *gin.Context) string {
			return c.ClientIP()
		}
	}
	if conf.TTL <= 0 {
		conf.TTL = defaultTTL
	}

	return func(c *gin.Context) {
		if conf.SkipFunc != nil && conf.SkipFunc(c) {
			return
		}

		key := conf.KeyFunc(c)
		limiter, err := limiterCache().Get(c.Request.Context(), key)
		if errors.Is(err, types.ErrEntryNotFound) {
			limiter = rate.NewLimiter(conf.Rate, conf.Burst)
			// The backend only rejects a negative lifetime, and conf.TTL is
			// forced positive above, so there is no failure to handle here.
			_ = limiterCache().Set(c.Request.Context(), key, limiter, conf.TTL)
		} else if err != nil {
			response.Abort(c, http.StatusBadRequest, "rate limiter unavailable")
			return
		}
		if !limiter.Allow() {
			if conf.OnLimitReached != nil {
				conf.OnLimitReached(c)
				c.Abort()
				return
			}
			response.Abort(c, http.StatusTooManyRequests, "too many requests")
			return
		}
	}
}
