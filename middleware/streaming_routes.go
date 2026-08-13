package middleware

import (
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// The streaming route registry names the routes whose responses are
// long-lived streams (Server-Sent Events). Request-scoped response treatment
// is wrong for those: capturing the response body would tee an unbounded
// stream, a circuit breaker would count a healthy connection as one
// never-returning request and starve its half-open probe budget, and a
// request timeout would tear the stream down mid-flight. The middlewares
// concerned consult the registry and leave streaming requests alone.
//
// Routes are marked at registration time (generated code marks routes their
// design declares as streaming; tests mark theirs by hand), keyed by method
// and the route pattern gin reports as FullPath.
var (
	streamingRouteMu sync.RWMutex
	streamingRoutes  = make(map[string]struct{})
)

// streamingRouteKey builds the registry key for one route.
func streamingRouteKey(method, path string) string {
	return strings.ToUpper(method) + " " + path
}

// MarkStreamingRoute registers a route as serving a long-lived streaming
// response. The path is the gin route pattern the handler is registered
// under, parameters included (e.g. "/api/items/:id/events").
func MarkStreamingRoute(method, path string) {
	streamingRouteMu.Lock()
	defer streamingRouteMu.Unlock()
	streamingRoutes[streamingRouteKey(method, path)] = struct{}{}
}

// IsStreamingRoute reports whether the route was marked as streaming.
func IsStreamingRoute(method, path string) bool {
	streamingRouteMu.RLock()
	defer streamingRouteMu.RUnlock()
	_, ok := streamingRoutes[streamingRouteKey(method, path)]
	return ok
}

// isStreamingRequest reports whether the request resolved to a streaming
// route. Requests gin matched to no route have an empty FullPath and are
// never streaming.
func isStreamingRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	route := c.FullPath()
	if route == "" {
		return false
	}
	return IsStreamingRoute(c.Request.Method, route)
}
