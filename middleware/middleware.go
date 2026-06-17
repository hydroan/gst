package middleware

import (
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/config"
	"github.com/sony/gobreaker"
	"go.uber.org/zap"
)

var (
	cb           *gobreaker.CircuitBreaker
	RouteManager *RouteParamsManager

	middlewareMu       sync.Mutex
	commonMiddlewares  []gin.HandlerFunc
	authMiddlewares    []gin.HandlerFunc
	applyCommonHandler func(gin.HandlerFunc)
	applyAuthHandler   func(gin.HandlerFunc)
)

// Register adds global middlewares that apply to all routes.
// Must be called before router.Init.
// Middlewares are auto-wrapped for tracing; name is inferred via reflection.
func Register(middlewares ...gin.HandlerFunc) {
	middlewareMu.Lock()
	defer middlewareMu.Unlock()

	for _, middleware := range middlewares {
		if middleware == nil {
			continue
		}
		// Automatically extract function name for tracing
		name := getFunctionName(middleware)
		// Automatically wrap middleware with tracing for performance monitoring
		wrapped := middlewareWrapper(name, middleware)
		zap.S().Infow("register common middleware", "name", name)
		commonMiddlewares = append(commonMiddlewares, wrapped)
		if applyCommonHandler != nil {
			applyCommonHandler(wrapped)
		}
	}
}

// RegisterAuth adds authentication/authorization middlewares.
// Must be called before router.Init.
// Middlewares are auto-wrapped for tracing; name is inferred via reflection.
func RegisterAuth(middlewares ...gin.HandlerFunc) {
	middlewareMu.Lock()
	defer middlewareMu.Unlock()

	for _, middleware := range middlewares {
		if middleware == nil {
			continue
		}
		// Automatically extract function name for tracing
		name := getFunctionName(middleware)
		// Automatically wrap middleware with tracing for performance monitoring
		wrapped := middlewareWrapper(name, middleware)
		zap.S().Infow("register auth middleware", "name", name)
		authMiddlewares = append(authMiddlewares, wrapped)
		if applyAuthHandler != nil {
			applyAuthHandler(wrapped)
		}
	}
}

// SetApplyHandlers installs the handlers used to attach registered middlewares to router groups.
// Existing registered middlewares are applied immediately in registration order.
func SetApplyHandlers(commonHandler, authHandler func(gin.HandlerFunc)) {
	middlewareMu.Lock()
	defer middlewareMu.Unlock()

	applyCommonHandler = commonHandler
	applyAuthHandler = authHandler

	if applyCommonHandler != nil {
		for _, middleware := range commonMiddlewares {
			applyCommonHandler(middleware)
		}
	}
	if applyAuthHandler != nil {
		for _, middleware := range authMiddlewares {
			applyAuthHandler(middleware)
		}
	}
}

func Init() (err error) {
	// Init circuit breaker
	cbCfg := config.App.Server.CircuitBreaker
	if cbCfg.MaxRequests == 0 {
		return errors.New("circuit breaker max_requests cannot be 0")
	}
	if cbCfg.MinRequests == 0 {
		return errors.New("circuit breaker min_requests cannot be 0")
	}
	if cbCfg.FailureRate <= 0 || cbCfg.FailureRate > 1 {
		return errors.New("circuit breaker failure_rate must be between 0 and 1")
	}

	cb = gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        cbCfg.Name,
		MaxRequests: cbCfg.MaxRequests,
		Interval:    cbCfg.Interval,
		Timeout:     cbCfg.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			if counts.Requests < cbCfg.MinRequests {
				return false
			}
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return failureRatio >= cbCfg.FailureRate
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			zap.S().Infow(
				"circuit breaker state changed",
				"name", name,
				"from", from.String(),
				"to", to.String(),
			)
		},
	})
	zap.S().Infow(
		"circuit breaker initialized",
		"name", cbCfg.Name,
		"max_requests", cbCfg.MaxRequests,
		"min_requests", cbCfg.MinRequests,
		"failure_rate", cbCfg.FailureRate,
		"interval", cbCfg.Interval,
		"timeout", cbCfg.Timeout,
	)

	// Init route params manager
	RouteManager = NewRouteParamsManager()

	return nil
}

// getFunctionName extracts the function name from a gin.HandlerFunc using reflection
func getFunctionName(fn gin.HandlerFunc) string {
	if fn == nil {
		return "unknown"
	}

	// Get the function pointer
	fnPtr := reflect.ValueOf(fn).Pointer()

	// Get function information from runtime
	fnInfo := runtime.FuncForPC(fnPtr)
	if fnInfo == nil {
		return "unknown"
	}

	// Get the full function name and location
	fullName := fnInfo.Name()
	file, line := fnInfo.FileLine(fnPtr)

	// Parse the function name
	// Example formats:
	// - package.FunctionName (regular function)
	// - package.Type.Method (method)
	// - package.FunctionName.func1 (closure inside FunctionName)
	// - package.glob..func1 (anonymous function at package level)

	// Remove package path, keep only the last part
	lastDot := strings.LastIndex(fullName, "/")
	if lastDot >= 0 {
		fullName = fullName[lastDot+1:]
	}

	// Split by dots to analyze structure
	parts := strings.Split(fullName, ".")
	if len(parts) < 2 {
		return cleanFunctionName(fullName)
	}

	// Get the last part (actual function/method name)
	funcName := parts[len(parts)-1]

	// Handle anonymous functions and closures
	if strings.HasPrefix(funcName, "func") || strings.Contains(funcName, "glob..func") {
		// Check if this is a closure from a named function
		if len(parts) >= 3 {
			// Check the parent context
			parentName := parts[len(parts)-2]

			// If parent is "glob" or starts with number, it's a package-level anonymous
			if parentName == "glob" || (len(parentName) > 0 && isNumeric(parentName[0])) {
				// Use file location for package-level anonymous functions
				if file != "" {
					return fmt.Sprintf("%s_L%d", filepath.Base(strings.TrimSuffix(file, ".go")), line)
				}
				return fmt.Sprintf("anonymous_L%d", line)
			}

			// If parent looks like a function name, use it
			// This handles cases like identifySession() returning a closure
			if parentName != "" && !strings.Contains(parentName, "..") {
				return parentName
			}
		}

		// Fallback to file and line for inline anonymous functions
		if file != "" {
			return fmt.Sprintf("%s_L%d", filepath.Base(strings.TrimSuffix(file, ".go")), line)
		}
		return "anonymous"
	}

	// Handle numbered functions (e.g., "1", "2" from init functions)
	if len(funcName) > 0 && isNumeric(funcName[0]) {
		if file != "" {
			return fmt.Sprintf("%s_L%d", filepath.Base(strings.TrimSuffix(file, ".go")), line)
		}
		return "func" + funcName
	}

	return cleanFunctionName(funcName)
}

// cleanFunctionName removes common suffixes and returns a clean function name
func cleanFunctionName(name string) string {
	// Remove method value suffix
	name = strings.TrimSuffix(name, "-fm")
	// Remove other potential suffixes
	name = strings.TrimSuffix(name, ".func1")
	name = strings.TrimSuffix(name, ".func2")
	return name
}

// isNumeric checks if a byte represents a numeric character
func isNumeric(b byte) bool {
	return b >= '0' && b <= '9'
}
