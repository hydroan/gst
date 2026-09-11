package ratelimiter

import (
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestNewConfigAppliesDefaults pins the limiter RateLimiter builds without
// options: a bucket of 50 tokens refilled every 100ms, keyed by client IP,
// skipping nothing — the same as WithBucket(50, 100*time.Millisecond).
func TestNewConfigAppliesDefaults(t *testing.T) {
	conf := newConfig(nil)

	require.Equal(t, 50, conf.capacity)
	require.Equal(t, 100*time.Millisecond, conf.refillEvery)
	require.NotNil(t, conf.keyFunc)
	require.Nil(t, conf.skipFunc)
}

// TestOptionsApplyValidValues pins that every option writes the value it is
// given into the config.
func TestOptionsApplyValidValues(t *testing.T) {
	conf := newConfig([]Option{
		WithBucket(3, time.Minute),
		WithKeyFunc(func(*gin.Context) string { return "sample" }),
		WithSkipFunc(func(*gin.Context) bool { return true }),
	})

	require.Equal(t, 3, conf.capacity)
	require.Equal(t, time.Minute, conf.refillEvery)
	require.Equal(t, "sample", conf.keyFunc(nil))
	require.True(t, conf.skipFunc(nil))
}

// TestWithBucketPanicsOnInvalidValues pins that a bucket which cannot be honored
// fails where it is written, while the longest refill time allowed still builds.
func TestWithBucketPanicsOnInvalidValues(t *testing.T) {
	invalid := map[string]func(){
		"zero capacity":              func() { WithBucket(0, time.Second) },
		"negative capacity":          func() { WithBucket(-1, time.Second) },
		"zero refill interval":       func() { WithBucket(1, 0) },
		"negative refill interval":   func() { WithBucket(1, -time.Second) },
		"refill beyond a century":    func() { WithBucket(2, maxRefillTime/2+time.Nanosecond) },
		"refill time that overflows": func() { WithBucket(math.MaxInt, time.Hour) },
	}
	for name, build := range invalid {
		t.Run(name, func(t *testing.T) {
			require.Panics(t, build)
		})
	}

	require.NotPanics(t, func() { WithBucket(2, maxRefillTime/2) })
}

// TestOptionsIgnoreNilFunctions pins that a nil key or skip function counts as
// not given, so the default stays in force.
func TestOptionsIgnoreNilFunctions(t *testing.T) {
	conf := newConfig([]Option{WithKeyFunc(nil), WithSkipFunc(nil)})

	require.NotNil(t, conf.keyFunc)
	require.Nil(t, conf.skipFunc)
}

// TestOptionsTolerateNilConfig pins that options are safe to apply to a nil
// config.
func TestOptionsTolerateNilConfig(t *testing.T) {
	require.NotPanics(t, func() {
		WithBucket(3, time.Minute)(nil)
		WithKeyFunc(func(*gin.Context) string { return "sample" })(nil)
		WithSkipFunc(func(*gin.Context) bool { return true })(nil)
	})
}

// TestRateLimiterIgnoresNilOption pins that a nil option in the variadic list
// is skipped rather than dereferenced.
func TestRateLimiterIgnoresNilOption(t *testing.T) {
	engine := newLimitedEngine(t, nil, WithBucket(1, slowRefill), WithKeyFunc(constantKey(t.Name())))

	require.Equal(t, http.StatusOK, probe(engine, "").Code)
	require.Equal(t, http.StatusTooManyRequests, probe(engine, "").Code)
}
