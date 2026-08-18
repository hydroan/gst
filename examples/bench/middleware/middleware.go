// Package middleware registers the application's custom HTTP middleware.
//
// middleware.Register applies to all routes; middleware.RegisterAuth applies
// only to routes behind authentication. Both take one or more gin.HandlerFunc
// and wrap each with tracing automatically.
//
// Example:
//
//	import (
//		"net/http"
//
//		"github.com/gin-gonic/gin"
//		"github.com/hydroan/gst/middleware"
//		"github.com/hydroan/gst/response"
//	)
//
//	func sample(c *gin.Context) {
//		// Runs before each handler. Refuse a request with response.Abort: it
//		// answers in the API envelope every other response carries, so one
//		// client reads them all the same way and can quote back the trace id
//		// that explains this one.
//		if c.GetHeader("X-Sample") == "" {
//			response.Abort(c, http.StatusForbidden, "sample header required")
//			return
//		}
//		c.Next()
//	}
//
//	func init() {
//		middleware.Register(sample)
//	}
package middleware

func init() {
	// TODO: register your custom middlewares here.
}
