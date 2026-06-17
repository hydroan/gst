package ratelimiter

import (
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// Option is a functional option for configuring RateLimiterConfig.
type Option func(*Config)

// WithRate sets the number of requests allowed per second.
// Non-positive values are ignored; the default (10 req/s) is used instead.
func WithRate(r rate.Limit) Option {
	return func(conf *Config) {
		if conf == nil || r <= 0 {
			return
		}
		conf.Rate = r
	}
}

// WithBurst sets the maximum burst size (token bucket capacity).
// Non-positive values are ignored; the default (50) is used instead.
func WithBurst(burst int) Option {
	return func(conf *Config) {
		if conf == nil || burst <= 0 {
			return
		}
		conf.Burst = burst
	}
}

// WithTTL sets the duration after which an idle rate limiter is evicted from cache.
// Non-positive values are ignored; the default (24h) is used instead.
func WithTTL(ttl time.Duration) Option {
	return func(conf *Config) {
		if conf == nil || ttl <= 0 {
			return
		}
		conf.TTL = ttl
	}
}

// WithKeyFunc sets the function used to extract a rate limit key from each request.
// A nil keyFunc is ignored; the default (client IP) is used instead.
func WithKeyFunc(keyFunc func(c *gin.Context) string) Option {
	return func(conf *Config) {
		if conf == nil || keyFunc == nil {
			return
		}
		conf.KeyFunc = keyFunc
	}
}

// WithSkipFunc sets a function that determines whether to skip rate limiting for a request.
// Returns true to bypass rate limiting (e.g. health check endpoints, internal IPs).
// A nil skipFunc is ignored.
func WithSkipFunc(skipFunc func(c *gin.Context) bool) Option {
	return func(conf *Config) {
		if conf == nil || skipFunc == nil {
			return
		}
		conf.SkipFunc = skipFunc
	}
}

// WithOnLimitReached sets a custom handler called when the rate limit is exceeded.
// The handler is responsible for writing the response; the default 429 response is skipped.
// A nil handler is ignored; the default CodeTooManyRequests response is used instead.
func WithOnLimitReached(onLimitReached gin.HandlerFunc) Option {
	return func(conf *Config) {
		if conf == nil || onLimitReached == nil {
			return
		}
		conf.OnLimitReached = onLimitReached
	}
}
