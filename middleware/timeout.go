package middleware

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	. "github.com/hydroan/gst/response"
	"go.uber.org/zap"
)

// Timeout returns a middleware that adds a timeout to the request context.
// If the request takes longer than the specified duration, it will be canceled.
//
// Parameters:
//   - timeout: Maximum duration for the request to complete
//
// Returns:
//   - A gin.HandlerFunc that enforces the timeout
//
// Example:
//
//	// Set 30 second timeout for all requests
//	router.Use(middleware.Timeout(30 * time.Second))
//
//	// Set 5 second timeout
//	router.Use(middleware.Timeout(5 * time.Second))
func Timeout(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Create a context with timeout
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		// Update request context
		c.Request = c.Request.WithContext(ctx)

		// Create a channel to signal completion
		done := make(chan struct{})
		panicChan := make(chan any, 1)

		go func() {
			defer func() {
				if r := recover(); r != nil {
					panicChan <- r
				}
			}()
			c.Next()
			close(done)
		}()

		// Wait for either completion, timeout, or panic
		select {
		case <-done:
			// Request completed successfully
		case p := <-panicChan:
			// Re-panic in the original goroutine so Recovery middleware can catch it
			// This ensures the panic is handled by the Recovery middleware and doesn't crash the server
			panic(p)
		case <-ctx.Done():
			// Request timed out
			if !c.Writer.Written() {
				zap.S().Warnw(
					"request timeout",
					"path", c.Request.URL.Path,
					"method", c.Request.Method,
					"timeout", timeout,
				)
				JSON(c, CodeContextTimeout)
				c.Abort()
			}
			// Cancel the context to signal handlers to stop
			cancel()
		}
	}
}
