package router

import (
	"context"
	"net"
	"net/http"
	gopath "path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/controller"
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

// APIPathPrefix is the base group all business API routes are mounted under.
const APIPathPrefix = "/api"

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

	ginParamPattern = regexp.MustCompile(`:([a-zA-Z0-9_]+)`)
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

// Routes returns a read-only snapshot of registered business API routes.
//
// Route parameters are converted from Gin's ":id" format to "{id}" so the
// returned paths can be reused by Casbin keyMatch3 policies and menu route
// bindings. Mutating the returned map or method slices does not affect router
// state.
func Routes() map[string][]string {
	snapshot := routesSnapshot()
	result := make(map[string][]string, len(snapshot))
	for endpoint, methods := range snapshot {
		result[normalizeRoutePath(endpoint)] = sortedHTTPMethods(methods)
	}
	return result
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

	base := root.Group(APIPathPrefix)
	auth = base.Group("")
	auth.Use(middleware.AuthMarker())
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
		if err := hook(Routes()); err != nil {
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

// Register registers HTTP routes for a model/action pair.
//
// Register is the public route registration entry point for generated code,
// modules, and advanced application code. It binds model metadata to
// framework-owned internal handlers; application code should use Register
// instead of importing the internal controller package.
//
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
//
// The raw route string is stamped into the controller config so factories can
// resolve the matching phase service through the route-derived registry key;
// it must therefore equal the route passed to the corresponding
// service.Register call. The config is shallow-copied first, keeping a
// caller-shared config safe for reuse across routes.
func Register[M types.Model, REQ types.Request, RSP types.Response](router gin.IRouter, route string, cfg *types.ControllerConfig[M], verbs ...consts.HTTPVerb) {
	if !validPath(route) {
		return
	}
	routed := types.ControllerConfig[M]{}
	if cfg != nil {
		routed = *cfg
	}
	routed.Route = route
	register[M, REQ, RSP](router, buildPath(route), buildVerbMap(verbs...), &routed)
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

	// Everything except the public route group is documented as requiring
	// authentication, which is the safe default for custom sub groups.
	authRequired := router != gin.IRouter(pub)

	if verbMap[consts.Create] {
		endpoint := gopath.Join(base, path)
		router.POST(path, controller.CreateFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodPost)
		middleware.RouteManager.Add(endpoint)
		go openapigen.Set[M, REQ, RSP](endpoint, authRequired, consts.Create)
	}
	if verbMap[consts.Delete] {
		endpoint := gopath.Join(base, path)
		router.DELETE(path, controller.DeleteFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodDelete)
		middleware.RouteManager.Add(endpoint)
		go openapigen.Set[M, REQ, RSP](endpoint, authRequired, consts.Delete)
	}
	if verbMap[consts.Update] {
		endpoint := gopath.Join(base, path)
		router.PUT(path, controller.UpdateFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodPut)
		middleware.RouteManager.Add(endpoint)
		go openapigen.Set[M, REQ, RSP](endpoint, authRequired, consts.Update)
	}
	if verbMap[consts.Patch] {
		endpoint := gopath.Join(base, path)
		router.PATCH(path, controller.PatchFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodPatch)
		middleware.RouteManager.Add(endpoint)
		go openapigen.Set[M, REQ, RSP](endpoint, authRequired, consts.Patch)
	}
	if verbMap[consts.List] {
		endpoint := gopath.Join(base, path)
		router.GET(path, controller.ListFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodGet)
		middleware.RouteManager.Add(endpoint)
		go openapigen.Set[M, REQ, RSP](endpoint, authRequired, consts.List)
	}

	if verbMap[consts.Get] {
		endpoint := gopath.Join(base, path)
		router.GET(path, controller.GetFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodGet)
		middleware.RouteManager.Add(endpoint)
		go openapigen.Set[M, REQ, RSP](endpoint, authRequired, consts.Get)
	}

	if verbMap[consts.CreateMany] {
		endpoint := gopath.Join(base, path)
		router.POST(path, controller.CreateManyFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodPost)
		middleware.RouteManager.Add(endpoint)
		go openapigen.Set[M, REQ, RSP](endpoint, authRequired, consts.CreateMany)
	}
	if verbMap[consts.DeleteMany] {
		endpoint := gopath.Join(base, path)
		router.DELETE(path, controller.DeleteManyFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodDelete)
		middleware.RouteManager.Add(endpoint)
		go openapigen.Set[M, REQ, RSP](endpoint, authRequired, consts.DeleteMany)
	}
	if verbMap[consts.UpdateMany] {
		endpoint := gopath.Join(base, path)
		router.PUT(path, controller.UpdateManyFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodPut)
		middleware.RouteManager.Add(endpoint)
		go openapigen.Set[M, REQ, RSP](endpoint, authRequired, consts.UpdateMany)
	}
	if verbMap[consts.PatchMany] {
		endpoint := gopath.Join(base, path)
		router.PATCH(path, controller.PatchManyFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodPatch)
		middleware.RouteManager.Add(endpoint)
		go openapigen.Set[M, REQ, RSP](endpoint, authRequired, consts.PatchMany)
	}

	if verbMap[consts.Import] {
		endpoint := gopath.Join(base, path)
		router.POST(path, controller.ImportFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodPost)
		middleware.RouteManager.Add(endpoint)
		go openapigen.Set[M, REQ, RSP](endpoint, authRequired, consts.Import)
	}
	if verbMap[consts.Export] {
		endpoint := gopath.Join(base, path)
		router.GET(path, controller.ExportFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodGet)
		middleware.RouteManager.Add(endpoint)
		go openapigen.Set[M, REQ, RSP](endpoint, authRequired, consts.Export)
	}
}

func validPath(route string) bool {
	route = strings.TrimSpace(route)
	if len(route) == 0 {
		zap.S().Warn("empty route, skip register routes")
		return false
	}
	return true
}

func registerRoute(endpoint, method string) {
	routeMu.Lock()
	defer routeMu.Unlock()

	routes[endpoint] = append(routes[endpoint], method)
}

func normalizeRoutePath(endpoint string) string {
	return ginParamPattern.ReplaceAllString(endpoint, `{$1}`)
}

func sortedHTTPMethods(methods []string) []string {
	seen := make(map[string]struct{}, len(methods))
	result := make([]string, 0, len(methods))
	for _, method := range methods {
		method = strings.ToUpper(strings.TrimSpace(method))
		if len(method) == 0 {
			continue
		}
		if _, ok := seen[method]; ok {
			continue
		}
		seen[method] = struct{}{}
		result = append(result, method)
	}
	sort.Slice(result, func(i int, j int) bool {
		left, leftOK := httpMethodRank(result[i])
		right, rightOK := httpMethodRank(result[j])
		if leftOK && rightOK {
			return left < right
		}
		if leftOK {
			return true
		}
		if rightOK {
			return false
		}
		return result[i] < result[j]
	})
	return result
}

func httpMethodRank(method string) (int, bool) {
	switch method {
	case http.MethodGet:
		return 0, true
	case http.MethodPost:
		return 1, true
	case http.MethodPut:
		return 2, true
	case http.MethodPatch:
		return 3, true
	case http.MethodDelete:
		return 4, true
	default:
		return 0, false
	}
}

// buildPath normalizes the API path.
func buildPath(path string) string {
	path = strings.TrimPrefix(path, APIPathPrefix+"/") // remove the '/api/' base prefix
	path = strings.TrimPrefix(path, "/")               // trim left "/"
	path = strings.TrimSuffix(path, "/")               // trim right "/"
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
