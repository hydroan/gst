package ratelimiter

import (
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/cache/freelru"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/response"
	"github.com/hydroan/gst/types"
	"golang.org/x/time/rate"
)

// The default bucket lets a key burst 50 requests and then keep up ten a
// second, the same as WithBucket(50, 100*time.Millisecond).
const (
	defaultCapacity    = 50
	defaultRefillEvery = 100 * time.Millisecond
)

// limiterCache holds one limiter per limiter instance and key.
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

// limiterInstances numbers the limiters RateLimiter builds. limiterCache is
// shared by the whole process, so each limiter keys its buckets under its own
// number: two limiters whose key functions agree — both keeping the client-IP
// default, say — would otherwise draw on one bucket, sized by whichever of them
// created it first. The number only has to be unique within the process,
// which is as far as the cache reaches.
var limiterInstances atomic.Uint64

// config describes the limiter one RateLimiter call builds.
type config struct {
	capacity    int
	refillEvery time.Duration
	keyFunc     func(*gin.Context) string
	skipFunc    func(*gin.Context) bool
}

// newConfig applies opts over the default limiter: the default bucket, keyed
// by client IP, skipping nothing. A nil option is ignored.
func newConfig(opts []Option) *config {
	conf := &config{
		capacity:    defaultCapacity,
		refillEvery: defaultRefillEvery,
		keyFunc:     requestctx.GinClientIP,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(conf)
		}
	}
	return conf
}

// RateLimiter returns a gin middleware that limits request rates per configurable key.
// Use functional options (WithBucket, WithKeyFunc, WithSkipFunc) to customize behavior;
// a request over the limit is refused with 429.
// Every call builds a limiter with buckets of its own, so two limiters never
// share a bucket even when their key functions return the same key.
//
// Example:
//
//	r.Use(ratelimiter.RateLimiter(
//	    ratelimiter.WithBucket(20, 100*time.Millisecond),
//	    ratelimiter.WithKeyFunc(func(c *gin.Context) string { return c.ClientIP() }),
//	    ratelimiter.WithSkipFunc(func(c *gin.Context) bool { return c.FullPath() == "/health" }),
//	))
func RateLimiter(opts ...Option) gin.HandlerFunc {
	conf := newConfig(opts)
	limit := rate.Every(conf.refillEvery)
	// A bucket left unused for capacity × refillEvery has refilled completely,
	// so dropping it then and handing its key a fresh one later changes
	// nothing: that is all the lifetime a bucket needs. The extra millisecond
	// covers the cache keeping expiry to the millisecond, and WithBucket bounds
	// the product, so it cannot overflow.
	idleLifetime := time.Duration(conf.capacity)*conf.refillEvery + time.Millisecond
	// Digits cannot contain the colon that ends them, so no key can make one
	// limiter's prefix read as another's.
	keyPrefix := strconv.FormatUint(limiterInstances.Add(1), 10) + ":"

	return func(c *gin.Context) {
		if conf.skipFunc != nil && conf.skipFunc(c) {
			return
		}

		key := keyPrefix + conf.keyFunc(c)
		limiter, err := limiterCache().Get(c.Request.Context(), key)
		if errors.Is(err, types.ErrEntryNotFound) {
			limiter = rate.NewLimiter(limit, conf.capacity)
		} else if err != nil {
			// The limiter is security state: a cache that cannot answer refuses
			// the request rather than switching the limit off.
			response.Abort(c, http.StatusInternalServerError, "rate limiter unavailable")
			return
		}
		// Every request restarts the bucket's lifetime, the refused ones too: a
		// lifetime counted from creation would hand a client hammering an empty
		// bucket a full one each time it ran out. The backend only rejects a
		// lifetime under a millisecond, and this one never is.
		_ = limiterCache().Set(c.Request.Context(), key, limiter, idleLifetime)
		if !limiter.Allow() {
			response.Abort(c, http.StatusTooManyRequests, "too many requests")
			return
		}
	}
}
