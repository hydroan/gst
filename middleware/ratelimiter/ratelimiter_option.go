package ratelimiter

import (
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
)

// maxRefillTime bounds how long an empty bucket may take to refill. The refill
// time is also how long an idle bucket is kept, so the bound keeps that
// lifetime clear of overflow; a bucket that would take longer to refill is a
// configuration mistake rather than a limit anyone means.
const maxRefillTime = 100 * 365 * 24 * time.Hour

// Option configures the limiter RateLimiter builds.
type Option func(*config)

// WithBucket sizes the token bucket every key gets: it holds up to capacity
// tokens, each request spends one, and one token comes back every refillEvery.
// A key can therefore burst capacity requests at once and then keep up one
// request per refillEvery.
//
// It panics when capacity or refillEvery is not positive, or when an empty
// bucket would take more than a century to refill, so a limit that cannot be
// honored fails where it is written instead of quietly limiting nothing.
func WithBucket(capacity int, refillEvery time.Duration) Option {
	if capacity <= 0 {
		panic(errors.Newf("ratelimiter: bucket capacity must be positive, got %d", capacity))
	}
	if refillEvery <= 0 {
		panic(errors.Newf("ratelimiter: bucket refill interval must be positive, got %s", refillEvery))
	}
	if refillEvery > maxRefillTime/time.Duration(capacity) {
		panic(errors.Newf("ratelimiter: a bucket of %d tokens refilled every %s takes more than a century to refill", capacity, refillEvery))
	}
	return func(conf *config) {
		if conf == nil {
			return
		}
		conf.capacity = capacity
		conf.refillEvery = refillEvery
	}
}

// WithKeyFunc sets how a request maps to the key whose bucket it spends. Keys
// only need to be unique within one limiter. A nil keyFunc is ignored and the
// default, the client IP, stays in force.
//
// Common keys:
//
//	c.ClientIP()                                 per client IP (default)
//	c.GetString("user_id")                       per authenticated user
//	c.FullPath()                                 per route
//	c.GetHeader("X-API-Key")                     per API key
//	c.FullPath() + ":" + c.GetString("user_id")  per user per route
func WithKeyFunc(keyFunc func(c *gin.Context) string) Option {
	return func(conf *config) {
		if conf == nil || keyFunc == nil {
			return
		}
		conf.keyFunc = keyFunc
	}
}

// WithSkipFunc sets a function that determines whether to skip rate limiting for a request.
// Returns true to bypass rate limiting (e.g. health check endpoints, internal IPs).
// A nil skipFunc is ignored.
func WithSkipFunc(skipFunc func(c *gin.Context) bool) Option {
	return func(conf *config) {
		if conf == nil || skipFunc == nil {
			return
		}
		conf.skipFunc = skipFunc
	}
}
