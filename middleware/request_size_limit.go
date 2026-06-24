package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/types/consts"
)

// RequestSizeLimit returns a middleware that limits the size of incoming request bodies.
// This helps prevent DoS attacks by limiting the amount of data that can be sent in a single request.
//
// Parameters:
//   - maxSize: Maximum allowed size in bytes for the request body
//
// Returns:
//   - A gin.HandlerFunc that enforces the request size limit
//
// Example:
//
//	// Limit request body to 10MB
//	router.Use(middleware.RequestSizeLimit(10 * 1024 * 1024))
//
//	// Limit request body to 1MB
//	router.Use(middleware.RequestSizeLimit(1024 * 1024))
func RequestSizeLimit(maxSize int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check Content-Length header first (if available and valid)
		if c.Request.ContentLength > 0 && c.Request.ContentLength > maxSize {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"code":            -1,
				"msg":             "request body too large",
				"data":            nil,
				consts.REQUEST_ID: c.GetString(consts.REQUEST_ID),
			})
			return
		}

		// Limit the request body reader (handles nil body case)
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSize)
		}
		c.Next()
	}
}
