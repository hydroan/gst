package middleware

import (
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/types/consts"
)

// RouteParams is a middleware to get route parameters
func RouteParams() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(consts.PARAMS, RouteManager.Get(c.FullPath()))
		c.Next()
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

func (rpm *RouteParamsManager) Add(path string) {
	path = strings.TrimSpace(path)
	if len(path) == 0 {
		return
	}
	rpm.mu.Lock()
	rpm.paramsMap[path] = rpm.parsePath(path)
	rpm.mu.Unlock()
}

func (rpm *RouteParamsManager) Get(path string) []string {
	rpm.mu.RLock()
	defer rpm.mu.RUnlock()
	val := rpm.paramsMap[path]
	if len(val) == 0 {
		// NOTE: {}string <nil> not deep equal to []string{}
		// map[key] returns {}string <nil> not []string{}
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
			// 处理 {id} 风格的参数 (如果需要)
			param := strings.TrimSuffix(strings.TrimPrefix(part, "{"), "}")
			if len(param) > 0 {
				params = append(params, param)
			}
		}
	}

	return params
}
