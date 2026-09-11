package middleware

import (
	"testing"

	"github.com/hydroan/gst/middleware/ratelimiter"
	"github.com/stretchr/testify/require"
)

// TestGetFunctionNameNamesClosureAfterItsConstructor pins the name a registered
// middleware is logged and traced under: a handler built by a constructor is a
// closure, and it takes the constructor's name rather than a closure index.
func TestGetFunctionNameNamesClosureAfterItsConstructor(t *testing.T) {
	require.Equal(t, "RateLimiter", getFunctionName(ratelimiter.RateLimiter()))
}
