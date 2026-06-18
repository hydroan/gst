package router

import (
	"context"
	"net"
	"net/http"
	gopath "path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/controller"
	"github.com/hydroan/gst/internal/openapigen"
	"github.com/hydroan/gst/middleware"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.uber.org/multierr"
	"go.uber.org/zap"
)

var (
	root *gin.Engine
	auth *gin.RouterGroup
	pub  *gin.RouterGroup

	server *http.Server

	started atomic.Uint32
	mu      sync.Mutex

	routeMu sync.RWMutex
	routes  = make(map[string][]string)

	routesReadyMu    sync.Mutex
	routesReadyHooks []func(routes map[string][]string) error
)

var globalErrors = make([]error, 0)

func routesSnapshot() map[string][]string {
	routeMu.RLock()
	defer routeMu.RUnlock()

	snapshot := make(map[string][]string, len(routes))
	for endpoint, methods := range routes {
		snapshot[endpoint] = append([]string(nil), methods...)
	}
	return snapshot
}

// OnRoutesReady registers a hook that runs after all routes are registered and before the server starts.
// The hook receives a route snapshot; mutating it does not change the router registry.
func OnRoutesReady(fn func(routes map[string][]string) error) {
	if fn == nil {
		return
	}

	routesReadyMu.Lock()
	defer routesReadyMu.Unlock()
	routesReadyHooks = append(routesReadyHooks, fn)
}

func Init() error {
	gin.SetMode(gin.ReleaseMode)
	root = gin.New()

	root.Use(
		middleware.Tracing(),
		middleware.Logger("api.log"),
		middleware.BodyLogger(),
		middleware.Recovery("recovery.log"),
		middleware.Cors(),
		middleware.RouteParams(),
		// middleware.Gzip(),
	)
	root.GET("/metrics", gin.WrapH(promhttp.Handler()))
	root.GET("/-/healthz", controller.Probe.Healthz)
	root.GET("/-/readyz", controller.Probe.Readyz)
	root.GET("/openapi.json", middleware.BaseAuth(), gin.WrapH(openapigen.DocumentHandler()))
	root.GET("/docs/*any", middleware.BaseAuth(), ginSwagger.WrapHandler(swaggerFiles.Handler, ginSwagger.URL("/openapi.json")))
	root.GET("/redoc", middleware.BaseAuth(), controller.Redoc)
	root.GET("/scalar", middleware.BaseAuth(), controller.Scalar)
	root.GET("/stoplight", middleware.BaseAuth(), controller.Stoplight)

	base := root.Group("/api")
	auth = base.Group("")
	pub = base.Group("")
	middleware.SetApplyHandlers(
		func(mid gin.HandlerFunc) {
			if started.Load() == 0 {
				auth.Use(mid)
				pub.Use(mid)
			}
		},
		func(mid gin.HandlerFunc) {
			if started.Load() == 0 {
				auth.Use(mid)
			}
		},
	)

	return nil
}

func Run() error {
	log := zap.S()
	if err := multierr.Combine(globalErrors...); err != nil {
		log.Error(err)
		return err
	}

	addr := net.JoinHostPort(config.App.Server.Listen, strconv.Itoa(config.App.Server.Port))
	for _, r := range root.Routes() {
		log.Debugw("", "method", r.Method, "path", r.Path)
	}

	routesReadyMu.Lock()
	hooks := append([]func(routes map[string][]string) error(nil), routesReadyHooks...)
	routesReadyMu.Unlock()

	for _, hook := range hooks {
		if err := hook(routesSnapshot()); err != nil {
			log.Errorw("failed to run routes ready hooks", "err", err)
			return err
		}
	}

	server = &http.Server{
		Addr:           addr,
		Handler:        root,
		ReadTimeout:    config.App.Server.ReadTimeout,
		WriteTimeout:   config.App.Server.WriteTimeout,
		IdleTimeout:    config.App.IdleTimeout,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	// mark the server as started.
	started.Store(1)
	log.Infow("backend server started", "addr", addr, "mode", config.App.Mode, "domain", config.App.Domain)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Errorw("failed to start server", "err", err)
		return err
	}
	return nil
}

func Auth() *gin.RouterGroup { return auth }
func Pub() *gin.RouterGroup  { return pub }

func Stop() {
	if server == nil {
		return
	}
	zap.S().Infow("backend server shutdown initiated")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		zap.S().Errorw("backend server shutdown failed", "err", err)
	} else {
		zap.S().Infow("backend server shutdown completed")
	}
	server = nil
}

// Register registers HTTP routes for a given model type with specified verbs
// using default controller configuration.
// It supports common CRUD operations along with import/export functionality.
//
// Parameters:
//   - router: The Gin router instance to register routes on
//   - path: Base path for the resource (automatically handles '/api/' prefix)
//   - verbs: Optional list of HTTPVerb to register. If empty, defaults to Most (basic CRUD operations)
//
// Route patterns registered:
//   - POST   /{path}         -> Create
//   - DELETE /{path}         -> Delete
//   - DELETE /{path}/:id     -> Delete
//   - PUT    /{path}         -> Update
//   - PUT    /{path}/:id     -> Update
//   - PATCH  /{path}         -> Patch
//   - PATCH  /{path}/:id     -> Patch
//   - GET    /{path}         -> List
//   - GET    /{path}/:id     -> Get
//   - POST   /{path}/import  -> Import
//   - GET    /{path}/export  -> Export
//   - POST   /{path}/batch   -> CreateMany
//   - DELETE /{path}/batch   -> DeleteMany
//   - PUT    /{path}/batch   -> UpdateMany
//   - PATCH  /{path}/batch   -> PatchMany
//
// For custom controller configuration, pass a ControllerConfig object.
func Register[M types.Model, REQ types.Request, RSP types.Response](router gin.IRouter, rawPath string, cfg *types.ControllerConfig[M], verbs ...consts.HTTPVerb) {
	if validPath(rawPath) {
		register[M, REQ, RSP](router, buildPath(rawPath), buildVerbMap(verbs...), cfg)
	}
}

func register[M types.Model, REQ types.Request, RSP types.Response](router gin.IRouter, path string, verbMap map[consts.HTTPVerb]bool, cfg ...*types.ControllerConfig[M]) {
	mu.Lock()
	defer mu.Unlock()

	// v := reflect.ValueOf(router).Elem()
	// base := v.FieldByName("basePath").String()
	var base string
	if group, ok := router.(*gin.RouterGroup); ok {
		base = group.BasePath()
	} else {
		panic("unknown router type")
	}

	if verbMap[consts.Create] {
		endpoint := gopath.Join(base, path)
		router.POST(path, controller.CreateFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodPost)
		middleware.RouteManager.Add(endpoint)
		go openapigen.Set[M, REQ, RSP](endpoint, consts.Create)
	}
	if verbMap[consts.Delete] {
		endpoint := gopath.Join(base, path)
		router.DELETE(path, controller.DeleteFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodDelete)
		middleware.RouteManager.Add(endpoint)
		go openapigen.Set[M, REQ, RSP](endpoint, consts.Delete)
	}
	if verbMap[consts.Update] {
		endpoint := gopath.Join(base, path)
		router.PUT(path, controller.UpdateFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodPut)
		middleware.RouteManager.Add(endpoint)
		go openapigen.Set[M, REQ, RSP](endpoint, consts.Update)
	}
	if verbMap[consts.Patch] {
		endpoint := gopath.Join(base, path)
		router.PATCH(path, controller.PatchFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodPatch)
		middleware.RouteManager.Add(endpoint)
		go openapigen.Set[M, REQ, RSP](endpoint, consts.Patch)
	}
	if verbMap[consts.List] {
		endpoint := gopath.Join(base, path)
		router.GET(path, controller.ListFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodGet)
		middleware.RouteManager.Add(endpoint)
		go openapigen.Set[M, REQ, RSP](endpoint, consts.List)
	}

	if verbMap[consts.Get] {
		endpoint := gopath.Join(base, path)
		router.GET(path, controller.GetFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodGet)
		middleware.RouteManager.Add(endpoint)
		go openapigen.Set[M, REQ, RSP](endpoint, consts.Get)
	}

	if verbMap[consts.CreateMany] {
		endpoint := gopath.Join(base, path)
		router.POST(path, controller.CreateManyFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodPost)
		middleware.RouteManager.Add(endpoint)
		go openapigen.Set[M, REQ, RSP](endpoint, consts.CreateMany)
	}
	if verbMap[consts.DeleteMany] {
		endpoint := gopath.Join(base, path)
		router.DELETE(path, controller.DeleteManyFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodDelete)
		middleware.RouteManager.Add(endpoint)
		go openapigen.Set[M, REQ, RSP](endpoint, consts.DeleteMany)
	}
	if verbMap[consts.UpdateMany] {
		endpoint := gopath.Join(base, path)
		router.PUT(path, controller.UpdateManyFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodPut)
		middleware.RouteManager.Add(endpoint)
		go openapigen.Set[M, REQ, RSP](endpoint, consts.UpdateMany)
	}
	if verbMap[consts.PatchMany] {
		endpoint := gopath.Join(base, path)
		router.PATCH(path, controller.PatchManyFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodPatch)
		middleware.RouteManager.Add(endpoint)
		go openapigen.Set[M, REQ, RSP](endpoint, consts.PatchMany)
	}

	if verbMap[consts.Import] {
		endpoint := gopath.Join(base, path)
		router.POST(path, controller.ImportFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodPost)
		middleware.RouteManager.Add(endpoint)
		go openapigen.Set[M, REQ, RSP](endpoint, consts.Import)
	}
	if verbMap[consts.Export] {
		endpoint := gopath.Join(base, path)
		router.GET(path, controller.ExportFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodGet)
		middleware.RouteManager.Add(endpoint)
		go openapigen.Set[M, REQ, RSP](endpoint, consts.Export)
	}
}

func validPath(rawPath string) bool {
	rawPath = strings.TrimSpace(rawPath)
	if len(rawPath) == 0 {
		zap.S().Warn("empty path, skip register routes")
		return false
	}
	return true
}

func registerRoute(endpoint, method string) {
	routeMu.Lock()
	defer routeMu.Unlock()

	routes[endpoint] = append(routes[endpoint], method)
}

// buildPath normalizes the API path.
func buildPath(path string) string {
	path = strings.TrimPrefix(path, `/api/`) // remove path prefix: '/api/'
	path = strings.TrimPrefix(path, "/")     // trim left "/"
	path = strings.TrimSuffix(path, "/")     // trim right "/"
	return "/" + path
}

// buildVerbMap creates a map of allowed HTTP verbs according to the specified verbs.
func buildVerbMap(verbs ...consts.HTTPVerb) map[consts.HTTPVerb]bool {
	verbMap := make(map[consts.HTTPVerb]bool)

	if len(verbs) == 0 {
		return make(map[consts.HTTPVerb]bool)
	}

	for _, verb := range verbs {
		verbMap[verb] = true
	}
	return verbMap
}
