package middleware

import (
	"math/rand/v2"
	"time"

	"github.com/gin-gonic/gin"
)

// Delay returns a middleware that adds a fixed delay before processing the request.
// This is primarily used for testing purposes to simulate network latency or slow responses.
//
// Parameters:
//   - duration: The delay duration to add before processing the request
//
// Returns:
//   - A gin.HandlerFunc that adds the specified delay
//
// Example, from an init function (see Register):
//
//	// Add a 100ms delay to every API route
//	middleware.Register(middleware.Delay(100 * time.Millisecond))
//
//	// Add a 1 second delay
//	middleware.Register(middleware.Delay(1 * time.Second))
func Delay(duration time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		time.Sleep(duration)
	}
}

// DelayRandom returns a middleware that adds a random delay within the specified range before processing the request.
// This is primarily used for testing purposes to simulate variable network latency or slow responses.
//
// Parameters:
//   - minDuration: The minimum delay duration
//   - maxDuration: The maximum delay duration
//
// Returns:
//   - A gin.HandlerFunc that adds a random delay between minDuration and maxDuration
//
// Example, from an init function (see Register):
//
//	// Add a random delay between 0 and 3000ms
//	middleware.Register(middleware.DelayRandom(0, 3000*time.Millisecond))
//
//	// Add a random delay between 100ms and 500ms
//	short, long := 100*time.Millisecond, 500*time.Millisecond
//	middleware.Register(middleware.DelayRandom(short, long))
func DelayRandom(minDuration, maxDuration time.Duration) gin.HandlerFunc {
	if minDuration < 0 {
		minDuration = 0
	}
	if maxDuration < minDuration {
		maxDuration = minDuration
	}
	rangeDuration := maxDuration - minDuration

	return func(c *gin.Context) {
		if rangeDuration > 0 {
			//nolint:gosec // G404: math/rand is sufficient for testing delay scenarios
			randomDelay := minDuration + time.Duration(rand.Int64N(int64(rangeDuration)))
			time.Sleep(randomDelay)
		} else if minDuration > 0 {
			time.Sleep(minDuration)
		}
	}
}

// DelayWithConfig returns a middleware that adds a configurable delay based on request properties.
// This allows for more flexible testing scenarios, such as different delays for different paths or methods.
//
// Parameters:
//   - delayFunc: A function that determines the delay duration based on the request context
//
// Returns:
//   - A gin.HandlerFunc that adds the delay determined by delayFunc
//
// Example, from an init function (see Register):
//
//	// Add a delay based on the path
//	byPath := func(c *gin.Context) time.Duration {
//		if strings.HasPrefix(c.Request.URL.Path, "/api/slow") {
//			return 2 * time.Second
//		}
//		return 100 * time.Millisecond
//	}
//	middleware.Register(middleware.DelayWithConfig(byPath))
func DelayWithConfig(delayFunc func(*gin.Context) time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		duration := delayFunc(c)
		if duration > 0 {
			time.Sleep(duration)
		}
	}
}
