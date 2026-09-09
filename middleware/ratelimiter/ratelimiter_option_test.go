package ratelimiter

import (
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

// TestOptionsApplyValidValues pins that every option writes the value it is
// given into the config.
func TestOptionsApplyValidValues(t *testing.T) {
	conf := new(Config)

	WithRate(rate.Limit(7))(conf)
	WithBurst(3)(conf)
	WithTTL(time.Minute)(conf)
	WithKeyFunc(func(*gin.Context) string { return "sample" })(conf)
	WithSkipFunc(func(*gin.Context) bool { return true })(conf)
	WithOnLimitReached(func(*gin.Context) {})(conf)

	require.InDelta(t, 7, float64(conf.Rate), 0)
	require.Equal(t, 3, conf.Burst)
	require.Equal(t, time.Minute, conf.TTL)
	require.NotNil(t, conf.KeyFunc)
	require.NotNil(t, conf.SkipFunc)
	require.NotNil(t, conf.OnLimitReached)
}

// TestOptionsIgnoreInvalidValues pins the guard every option carries: a value
// that cannot be honored leaves the config untouched, so RateLimiter fills in
// its default instead of running on a value nobody could have meant.
func TestOptionsIgnoreInvalidValues(t *testing.T) {
	t.Run("non-positive numbers", func(t *testing.T) {
		conf := &Config{Rate: rate.Limit(7), Burst: 3, TTL: time.Minute}

		WithRate(0)(conf)
		WithRate(-1)(conf)
		WithBurst(0)(conf)
		WithBurst(-1)(conf)
		WithTTL(0)(conf)
		WithTTL(-time.Second)(conf)

		require.InDelta(t, 7, float64(conf.Rate), 0)
		require.Equal(t, 3, conf.Burst)
		require.Equal(t, time.Minute, conf.TTL)
	})

	t.Run("nil functions", func(t *testing.T) {
		conf := new(Config)

		WithKeyFunc(nil)(conf)
		WithSkipFunc(nil)(conf)
		WithOnLimitReached(nil)(conf)

		require.Nil(t, conf.KeyFunc)
		require.Nil(t, conf.SkipFunc)
		require.Nil(t, conf.OnLimitReached)
	})
}

// TestOptionsTolerateNilConfig pins that options are safe to apply to a nil
// config, which is what keeps RateLimiter's option loop from panicking on one.
func TestOptionsTolerateNilConfig(t *testing.T) {
	require.NotPanics(t, func() {
		WithRate(rate.Limit(7))(nil)
		WithBurst(3)(nil)
		WithTTL(time.Minute)(nil)
		WithKeyFunc(func(*gin.Context) string { return "sample" })(nil)
		WithSkipFunc(func(*gin.Context) bool { return true })(nil)
		WithOnLimitReached(func(*gin.Context) {})(nil)
	})
}

// TestRateLimiterIgnoresNilOption pins that a nil option in the variadic list
// is skipped rather than dereferenced.
func TestRateLimiterIgnoresNilOption(t *testing.T) {
	engine := newLimitedEngine(t, nil, WithBurst(1), WithKeyFunc(constantKey(t.Name())))

	require.Equal(t, http.StatusOK, probe(engine, "").Code)
	require.Equal(t, http.StatusTooManyRequests, probe(engine, "").Code)
}
