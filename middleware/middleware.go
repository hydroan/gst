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
	"github.com/hydroan/gst/internal/middleware"
)

// Register adds middlewares that run on every API route. Call it from an init
// function: a route registered before the call runs without the middleware,
// and a call once the server runs panics, the routes having their chains
// already. Registered that way, they run in registration order, and on the
// routes of router.Auth ahead of every middleware RegisterAuth adds. When
// tracing is enabled, each middleware runs in a span of its own.
func Register(middlewares ...gin.HandlerFunc) {
	middleware.Register(middlewares...)
}

// RegisterAuth adds middlewares that run only on the routes registered on
// router.Auth: the place for authentication and authorization. Call it from an
// init function: a route registered before the call runs without the
// middleware, and a call once the server runs panics, the routes having their
// chains already. Registered that way, they run in registration order, after
// every middleware Register adds. When tracing is enabled, each middleware runs
// in a span of its own.
func RegisterAuth(middlewares ...gin.HandlerFunc) {
	middleware.RegisterAuth(middlewares...)
}

// CircuitBreaker returns a middleware that runs each request through the
// circuit breaker configured under server.circuit_breaker, which the framework
// builds as it bootstraps. A request counts as failed when its handler writes
// a response with a 5xx status, or writes nothing and records an error. Once
// enough requests have been counted and the configured share of them failed,
// the breaker opens: requests are refused with 503 until its timeout has
// passed and trial requests succeed again. A request the breaker lets through
// keeps the answer its handlers gave, failed or not. Requests to streaming
// routes bypass the breaker.
func CircuitBreaker() gin.HandlerFunc {
	return middleware.CircuitBreaker()
}
