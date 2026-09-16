// Package middleware is the HTTP middleware a project mounts: Register adds
// middleware to every API route, RegisterAuth to the routes of the
// authenticated group, and the constructors here build the middleware the
// framework ships for either. IAMSession and Authz are the middleware of the
// iam and authz modules, whose source files gg module copy copies into a
// project.
//
// The chain the framework mounts ahead of every route itself, and the
// machinery that mounts registered middleware onto the routes, are the
// framework's own.
package middleware

import (
	"github.com/gin-gonic/gin"
	internalmiddleware "github.com/hydroan/gst/internal/middleware"
	"go.opentelemetry.io/otel/trace"
)

// Register adds middlewares that run on every API route. Call it from an init
// function: a route registered before the call runs without the middleware.
// Registered that way, they run in registration order, and on the routes of
// router.Auth ahead of every middleware RegisterAuth adds. When tracing is
// enabled, each middleware runs in a span of its own.
func Register(middlewares ...gin.HandlerFunc) {
	internalmiddleware.Register(middlewares...)
}

// RegisterAuth adds middlewares that run only on the routes registered on
// router.Auth: the place for authentication and authorization. Call it from an
// init function: a route registered before the call runs without the
// middleware. Registered that way, they run in registration order, after every
// middleware Register adds. When tracing is enabled, each middleware runs in a
// span of its own.
func RegisterAuth(middlewares ...gin.HandlerFunc) {
	internalmiddleware.RegisterAuth(middlewares...)
}

// CircuitBreaker returns a middleware that runs each request through the
// circuit breaker configured under server.circuit_breaker, which the framework
// builds as it bootstraps. A request counts as failed when its handler writes
// a response with a 5xx status, or writes nothing and records an error. Once
// enough requests have been counted and the configured share of them failed,
// the breaker opens: requests are refused with 503 until its timeout has
// passed and trial requests succeed again. Requests to streaming routes bypass
// the breaker.
func CircuitBreaker() gin.HandlerFunc {
	return internalmiddleware.CircuitBreaker()
}

// GetSpanFromContext returns the server span the framework's tracing opened
// for the request or, when there is none, the span current in the request
// context.
func GetSpanFromContext(c *gin.Context) trace.Span {
	return internalmiddleware.GetSpanFromContext(c)
}

// RecordError records err on the span GetSpanFromContext returns, when that
// span is recording.
func RecordError(c *gin.Context, err error) {
	internalmiddleware.RecordError(c, err)
}
