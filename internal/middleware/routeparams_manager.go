package middleware

import (
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/requestctx"
)

// routeParams returns the middleware that stamps on the gin context the
// names of the path parameters of the route the request matched.
func routeParams() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(requestctx.KeyParams, RouteParamNames.Get(c.FullPath()))
	}
}

// RouteParamsManager holds parsed route path parameters for middleware.
type RouteParamsManager struct {
	paramsMap map[string][]string
	mu        sync.RWMutex
}

// NewRouteParamsManager returns a new RouteParamsManager.
func NewRouteParamsManager() *RouteParamsManager {
	return &RouteParamsManager{
		paramsMap: make(map[string][]string),
	}
}

// Add records the names of the path parameters of path, a route pattern
// such as /records/:id.
func (rpm *RouteParamsManager) Add(path string) {
	path = strings.TrimSpace(path)
	if len(path) == 0 {
		return
	}
	rpm.mu.Lock()
	rpm.paramsMap[path] = rpm.parsePath(path)
	rpm.mu.Unlock()
}

// Get returns the parameter names Add recorded for path, and an empty slice
// for a path that has none.
func (rpm *RouteParamsManager) Get(path string) []string {
	rpm.mu.RLock()
	defer rpm.mu.RUnlock()
	val := rpm.paramsMap[path]
	if len(val) == 0 {
		// A missing key reads as a nil slice, which is not deeply equal to
		// an empty one, so a path with no parameters answers []string{}.
		return []string{}
	}
	return val
}

func (rpm *RouteParamsManager) parsePath(path string) []string {
	parts := strings.Split(path, "/")
	var params []string

	for _, part := range parts {
		if after, ok := strings.CutPrefix(part, ":"); ok {
			param := after
			if len(param) > 0 {
				params = append(params, param)
			}
		} else if strings.Contains(part, "{") && strings.Contains(part, "}") {
			// handle {id} style parameters
			param := strings.TrimSuffix(strings.TrimPrefix(part, "{"), "}")
			if len(param) > 0 {
				params = append(params, param)
			}
		}
	}

	return params
}
