// Package router builds the HTTP server a gst process serves: the engine
// carrying the framework's middleware chain and operational endpoints, the
// route groups registered routes attach to, the routes-ready hooks, and the
// server's start and shutdown. The public router package forwards the part a
// project registers routes and hooks through.
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
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/controller"
	"github.com/hydroan/gst/internal/middleware"
	"github.com/hydroan/gst/internal/openapigen"
	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/sse"
	"github.com/hydroan/gst/internal/types"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
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
	routesReadyHooks []func(ctx context.Context, routes map[string][]string) error

	ginParamPattern = regexp.MustCompile(`:([a-zA-Z0-9_]+)`)
)

func routesSnapshot() map[string][]string {
	routeMu.RLock()
	defer routeMu.RUnlock()

	snapshot := make(map[string][]string, len(routes))
	for endpoint, methods := range routes {
		snapshot[endpoint] = append([]string(nil), methods...)
	}
	return snapshot
}

// Routes returns a snapshot of the registered API routes, keyed by path with
// parameters written "{id}"; the public router.Routes forwards to it and
// documents the contract.
func Routes() map[string][]string {
	snapshot := routesSnapshot()
	result := make(map[string][]string, len(snapshot))
	for endpoint, methods := range snapshot {
		result[normalizeRoutePath(endpoint)] = sortedHTTPMethods(methods)
	}
	return result
}

// OnRoutesReady registers fn to run by RunRoutesReadyHooks; a nil fn is
// ignored. The public router.OnRoutesReady forwards to it and documents the
// contract a hook runs under.
func OnRoutesReady(fn func(ctx context.Context, routes map[string][]string) error) {
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
	if err := applyTrustedProxies(root); err != nil {
		return err
	}

	root.Use(middleware.Builtin()...)
	// A request matching no route is answered in the envelope like every other
	// refusal, instead of gin's plain-text default — which carries no code and
	// no trace id, so a client parsing the documented shape cannot tell it from
	// a malformed response.
	root.NoRoute(func(c *gin.Context) {
		response.Abort(c, http.StatusNotFound, "not found")
	})

	// A path this server does serve, asked for with a method it does not, is
	// answered as method not allowed rather than folded into not found. gin
	// fills in the Allow header naming the methods that path accepts, which is
	// what turns the caller's mistake into something it can read: a PUT sent to
	// a route that only takes POST says exactly that, instead of looking like a
	// mistyped path and sending someone to re-check the route table.
	//
	// Separating the two discloses nothing. /openapi.json is served on this
	// same port without credentials and lists every route registered here, so
	// the path a method-not-allowed confirms is already published; withholding
	// the distinction would cost the caller its diagnosis and keep no secret.
	//
	// The extra lookup gin does to find those methods runs only on requests
	// that matched nothing, never on a request that reached a handler.
	root.HandleMethodNotAllowed = true
	root.NoMethod(func(c *gin.Context) {
		response.Abort(c, http.StatusMethodNotAllowed, "method not allowed")
	})

	// A request is matched against its path exactly as sent. gin can loosen
	// that three ways, and all three stay off — set here explicitly rather than
	// left to gin's defaults, so the reason sits beside the setting and an
	// upgrade to gin cannot quietly change what a request resolves to.
	//
	// RedirectTrailingSlash would answer "/items/" with a redirect to "/items".
	// gin writes that redirect from its router, before any handler is chosen,
	// and never runs the middleware chain for it: the redirect goes out with no
	// access log line, no metrics, no trace id and no CORS headers. A
	// cross-origin browser request therefore receives a redirect lacking
	// Access-Control-Allow-Origin and the browser refuses it, so the convenience
	// fails exactly the clients most likely to send such a path. With it off the
	// request is not found, answered through the whole chain with everything a
	// refusal carries.
	//
	// RedirectFixedPath would correct letter case and stray path segments and
	// redirect to the result. It takes the same redirect branch with the same
	// blind spot, and it would match paths case-insensitively where URLs are
	// defined to be case-sensitive.
	//
	// RemoveExtraSlash differs in kind: it cleans the path before matching and
	// then serves the request, so "//items//1" would be answered as "/items/1"
	// rather than redirected there. That serves one route under several
	// spellings, and a rule a proxy or gateway applies to the path as written
	// does not match the other spellings the application would still answer.
	root.RedirectTrailingSlash = false
	root.RedirectFixedPath = false
	root.RemoveExtraSlash = false

	// The operational endpoints, in one class: they are not business API, they
	// carry no authentication, and what protects them is the network rather
	// than this process. This is deliberate — reviewers reaching for the
	// missing auth check should read the rest of this comment first.
	//
	// Everything registered under the API prefix below answers only an
	// authenticated caller. These do not, because their readers hold no account
	// here: an orchestrator deciding whether to route traffic to this instance,
	// a metrics scraper, and people integrating against this service. An
	// authentication this server enforced would be one every such reader has to
	// be handed credentials for, and a credential the framework ships a default
	// for is worse than none at all — it protects nothing while reading, to
	// anyone reviewing this file, as though it did.
	//
	// The boundary is the deployment's. Under Kubernetes it falls out of the
	// topology rather than out of configuration: a scraper reaches the pod's
	// port directly, while the Ingress routes only the paths it is given, and
	// these are not among them — route just the API prefix and this class is
	// unreachable from outside the cluster without anyone having to remember a
	// rule. A deployment that instead publishes this whole port at a public
	// address publishes everything below with it.
	//
	// So that the decision can be made knowingly, what each one discloses:
	//
	//   /-/healthz, /-/readyz  Whether this process is alive, and whether it is
	//                          draining. Nothing else; the readiness body is
	//                          fixed text precisely because this is unguarded.
	//   /metrics               The routes that have actually been served, as gin
	//                          route patterns, with their request counts and
	//                          latencies by status; the database table names
	//                          behind the cache counters; process memory, CPU,
	//                          uptime and build info.
	//   /openapi.json          Every route this server registers, with the
	//                          request and response models of each.
	//   /docs                  Swagger UI rendering that same document, and
	//                          nothing beyond it.
	root.GET("/metrics", gin.WrapH(promhttp.Handler()))
	root.GET("/-/healthz", controller.Probe.Healthz)
	root.GET("/-/readyz", controller.Probe.Readyz)
	root.GET("/openapi.json", gin.WrapH(openapigen.DocumentHandler()))
	// The document has one rendering, and it is this one because its assets are
	// compiled into the binary. It therefore renders in a cluster with no egress,
	// and it runs no script fetched at page load from a third party — a page
	// served without credentials is the last place to run code a CDN can change
	// underneath it.
	root.GET("/docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, ginSwagger.URL("/openapi.json")))

	base := root.Group(consts.APIPathPrefix)
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

// RunRoutesReadyHooks runs the hooks OnRoutesReady registered, in
// registration order, on ctx, and returns the first error — ctx ending
// between two hooks included, reported as its cause, so a stop reaches a
// hook before it starts as well as during one. Bootstrap's Run calls it once every route is
// registered and every table exists, before the components that run
// alongside the server start and before the listener opens: what the hooks
// seed is there for the first round of a job and for the first request
// alike.
func RunRoutesReadyHooks(ctx context.Context) error {
	routesReadyMu.Lock()
	hooks := append([]func(context.Context, map[string][]string) error(nil), routesReadyHooks...)
	routesReadyMu.Unlock()

	for _, hook := range hooks {
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}
		if err := hook(ctx, Routes()); err != nil {
			// A hook cut short by the context ending is the stop's doing,
			// not a failure of the hook.
			if ctx.Err() == nil {
				zap.S().Errorw("failed to run routes ready hooks", "err", err)
			}
			return err
		}
	}
	return nil
}

func Run() error {
	log := zap.S()
	addr := net.JoinHostPort(config.App.Server.Listen, strconv.Itoa(config.App.Server.Port))
	for _, r := range root.Routes() {
		log.Debugw("", "method", r.Method, "path", r.Path)
	}

	server = newServer(addr, root)

	// mark the server as started.
	started.Store(1)
	log.Infow("backend server started", "addr", addr, "mode", config.App.Mode, "domain", config.App.Domain)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Errorw("failed to start server", "err", err)
		return err
	}
	return nil
}

// newServer builds the HTTP server Run serves handler on at addr. A
// Server-Sent Events stream on it ends the moment the server begins to shut
// down: Shutdown waits for every active request and cancels none of their
// contexts, so a stream watching only its request would hold the shutdown for
// as long as its client stays, up to the bound Stop gives it. Every request
// context carries the server's shutdown signal for the stream to watch, see
// sse.StreamContext; ordinary requests do not watch it and run to completion.
func newServer(addr string, handler http.Handler) *http.Server {
	shutdown, beginShutdown := context.WithCancel(context.Background())
	srv := &http.Server{
		Addr:           addr,
		Handler:        handler,
		ReadTimeout:    config.App.Server.ReadTimeout,
		WriteTimeout:   config.App.Server.WriteTimeout,
		IdleTimeout:    config.App.IdleTimeout,
		MaxHeaderBytes: 1 << 20, // 1 MB
		BaseContext: func(net.Listener) context.Context {
			return sse.WithServerShutdown(context.Background(), shutdown)
		},
	}
	srv.RegisterOnShutdown(beginShutdown)
	return srv
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

// Register registers route on the router group for each of verbs, each served
// by the controller factory of that verb, and records it for Routes, the
// route parameter registry and the OpenAPI document; the public
// router.Register forwards to it and documents the contract.
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
		openapigen.Set[M, REQ, RSP](endpoint, authRequired, consts.Create)
	}
	if verbMap[consts.Delete] {
		endpoint := gopath.Join(base, path)
		router.DELETE(path, controller.DeleteFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodDelete)
		middleware.RouteManager.Add(endpoint)
		openapigen.Set[M, REQ, RSP](endpoint, authRequired, consts.Delete)
	}
	if verbMap[consts.Update] {
		endpoint := gopath.Join(base, path)
		router.PUT(path, controller.UpdateFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodPut)
		middleware.RouteManager.Add(endpoint)
		openapigen.Set[M, REQ, RSP](endpoint, authRequired, consts.Update)
	}
	if verbMap[consts.Patch] {
		endpoint := gopath.Join(base, path)
		router.PATCH(path, controller.PatchFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodPatch)
		middleware.RouteManager.Add(endpoint)
		openapigen.Set[M, REQ, RSP](endpoint, authRequired, consts.Patch)
	}
	if verbMap[consts.List] {
		endpoint := gopath.Join(base, path)
		router.GET(path, controller.ListFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodGet)
		middleware.RouteManager.Add(endpoint)
		openapigen.Set[M, REQ, RSP](endpoint, authRequired, consts.List)
	}

	if verbMap[consts.Get] {
		endpoint := gopath.Join(base, path)
		router.GET(path, controller.GetFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodGet)
		middleware.RouteManager.Add(endpoint)
		openapigen.Set[M, REQ, RSP](endpoint, authRequired, consts.Get)
	}

	if verbMap[consts.CreateMany] {
		endpoint := gopath.Join(base, path)
		router.POST(path, controller.CreateManyFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodPost)
		middleware.RouteManager.Add(endpoint)
		openapigen.Set[M, REQ, RSP](endpoint, authRequired, consts.CreateMany)
	}
	if verbMap[consts.DeleteMany] {
		endpoint := gopath.Join(base, path)
		router.DELETE(path, controller.DeleteManyFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodDelete)
		middleware.RouteManager.Add(endpoint)
		openapigen.Set[M, REQ, RSP](endpoint, authRequired, consts.DeleteMany)
	}
	if verbMap[consts.UpdateMany] {
		endpoint := gopath.Join(base, path)
		router.PUT(path, controller.UpdateManyFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodPut)
		middleware.RouteManager.Add(endpoint)
		openapigen.Set[M, REQ, RSP](endpoint, authRequired, consts.UpdateMany)
	}
	if verbMap[consts.PatchMany] {
		endpoint := gopath.Join(base, path)
		router.PATCH(path, controller.PatchManyFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodPatch)
		middleware.RouteManager.Add(endpoint)
		openapigen.Set[M, REQ, RSP](endpoint, authRequired, consts.PatchMany)
	}

	if verbMap[consts.Import] {
		endpoint := gopath.Join(base, path)
		router.POST(path, controller.ImportFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodPost)
		middleware.RouteManager.Add(endpoint)
		openapigen.Set[M, REQ, RSP](endpoint, authRequired, consts.Import)
	}
	if verbMap[consts.Export] {
		endpoint := gopath.Join(base, path)
		router.GET(path, controller.ExportFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodGet)
		middleware.RouteManager.Add(endpoint)
		openapigen.Set[M, REQ, RSP](endpoint, authRequired, consts.Export)
	}

	if verbMap[consts.SSE] {
		endpoint := gopath.Join(base, path)
		router.GET(path, controller.SSEFactory[M, REQ, RSP](cfg...))
		registerRoute(endpoint, http.MethodGet)
		middleware.RouteManager.Add(endpoint)
		// Streaming responses are exempt from request-scoped response
		// treatment (body capture, circuit breaking, request timeouts); the
		// registry is how the middlewares concerned recognize them.
		middleware.MarkStreamingRoute(http.MethodGet, endpoint)
		openapigen.Set[M, REQ, RSP](endpoint, authRequired, consts.SSE)
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
	path = strings.TrimPrefix(path, consts.APIPathPrefix+"/") // remove the '/api/' base prefix
	path = strings.TrimPrefix(path, "/")                      // trim left "/"
	path = strings.TrimSuffix(path, "/")                      // trim right "/"
	return "/" + path
}

// buildVerbMap creates a map of allowed HTTP verbs according to the specified verbs.
func buildVerbMap(verbs ...consts.HTTPVerb) map[consts.HTTPVerb]bool {
	verbMap := make(map[consts.HTTPVerb]bool, len(verbs))
	for _, verb := range verbs {
		verbMap[verb] = true
	}
	return verbMap
}
