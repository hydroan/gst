package middleware

import (
	"fmt"
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/response"
	"github.com/sony/gobreaker"
	"go.uber.org/zap"
)

// CircuitBreaker returns the middleware that runs each request through the
// breaker Init builds; the public middleware.CircuitBreaker forwards to it and
// documents the contract. It lives beside the breaker rather than in the
// public package because the breaker is built from configuration as the
// framework starts.
func CircuitBreaker() gin.HandlerFunc {
	return func(c *gin.Context) {
		// A streaming request holds its connection open for as long as the
		// client listens; counted as an in-flight request it would sit in the
		// breaker's counts forever and, worse, occupy the half-open probe
		// budget so the breaker never closes again.
		if IsStreamingRequest(c) {
			return
		}

		// Get request info for better logging
		path := c.Request.URL.Path
		method := c.Request.Method

		_, err := cb.Execute(func() (any, error) {
			c.Next()

			if c.Writer.Written() {
				if c.Writer.Status() >= 500 {
					return nil, fmt.Errorf("server error: %d, path: %s, method: %s",
						c.Writer.Status(), path, method)
				}
				return nil, nil
			}

			if len(c.Errors) > 0 {
				return nil, fmt.Errorf("gin errors: %s, path: %s, method: %s",
					c.Errors.String(), path, method)
			}

			return nil, nil
		})
		// Only a request the breaker refused is the breaker's to answer: its
		// handlers never ran. A request the breaker let through was answered by
		// its handlers, or left unanswered by them, and its failure only counts
		// against the breaker; writing here would append a second envelope to
		// an answer already sent.
		if !errors.Is(err, gobreaker.ErrOpenState) && !errors.Is(err, gobreaker.ErrTooManyRequests) {
			return
		}

		zap.S().Errorw(
			"circuit breaker error",
			"error", err.Error(),
			"path", path,
			"method", method,
		)

		// The caller is told the one thing it can act on, which is to try again
		// later.
		response.Abort(c, http.StatusServiceUnavailable, "service unavailable")
	}
}
