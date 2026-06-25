package ratelimiter

import (
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/cache/ristretto"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"golang.org/x/time/rate"
)

const (
	defaultRate  = rate.Limit(10) // 默认每秒允许 10 个请求
	defaultBurst = 50             // 默认令牌桶容量
	defaultTTL   = 24 * time.Hour // 默认限流器过期时间
)

func init() {
	if err := ristretto.Init(); err != nil {
		panic(err)
	}
}

var ratelimiterMap = ristretto.Cache[*rate.Limiter]()

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
			c.Next()
			return
		}

		key := conf.KeyFunc(c)
		limiter, err := ratelimiterMap.Get(key)
		if errors.Is(err, types.ErrEntryNotFound) {
			limiter = rate.NewLimiter(conf.Rate, conf.Burst)
			_ = ratelimiterMap.Set(key, limiter, conf.TTL)
		} else if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"code":          -1,
				"msg":           "rate limiter unavailable",
				"data":          nil,
				consts.TRACE_ID: c.GetString(consts.TRACE_ID),
			})
			return
		}
		if !limiter.Allow() {
			if conf.OnLimitReached != nil {
				conf.OnLimitReached(c)
				c.Abort()
				return
			}
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code":          -1,
				"msg":           "too many requests",
				"data":          nil,
				consts.TRACE_ID: c.GetString(consts.TRACE_ID),
			})
			return
		}
		c.Next()
	}
}
