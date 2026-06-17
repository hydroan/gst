package middleware

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
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

			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error":  http.StatusText(http.StatusServiceUnavailable),
				"detail": err.Error(),
			})
		}
	}
}
