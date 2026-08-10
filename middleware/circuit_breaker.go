package middleware

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/response"
	"go.uber.org/zap"
)

func CircuitBreaker() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get request info for better logging
		path := c.Request.URL.Path
		method := c.Request.Method

		if _, err := cb.Execute(func() (any, error) {
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
		}); err != nil {
			if c.Writer.Written() && c.Writer.Status() < 500 {
				return
			}

			// Log circuit breaker error
			zap.S().Errorw(
				"circuit breaker error",
				"error", err.Error(),
				"path", path,
				"method", method,
			)

			// The error describes this server to itself — the status a handler
			// wrote, the path it wrote it for, the gin errors behind it — and
			// the log above already holds it. The caller is told the one thing
			// it can act on, which is to try again later.
			response.Abort(c, http.StatusServiceUnavailable, "service unavailable")
		}
	}
}
