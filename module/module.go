// Package module provides a unified module registration system that automatically
// registers models, services, and HTTP routes for CRUD operations.
//
// A module consists of three components:
//   - Model: Database entity implementing types.Model
//   - Service: Business logic implementing types.Service
//   - Module: Configuration implementing types.Module
//
// Usage:
//  1. Define model (embedding model.Base), request/response types, and service (embedding service.Base)
//  2. Implement module with types.Module interface
//  3. Call module.Use() with desired route options
//
// Example:
//
//	module.Use[*User, *UserReq, *UserRsp](
//	    &UserModule{},
//	    module.CRUD(
//	        service.Create,
//	        service.List,
//	        service.Get,
//	    ),
//	)
//
// Route paths are normalized (leading slashes and "api/" prefix are removed).
// Authentication is controlled by Module.Pub() method.
//
// See module/helloworld for complete examples.
package module

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/router"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/internal/types"
)

var (
	notify      = make(chan struct{})
	initialized atomic.Bool

	registerMu      sync.Mutex
	registerCond    = sync.NewCond(&registerMu)
	pendingRegister int
)

type useRouteMode int

const (
	useRouteModeCRUD useRouteMode = iota
	useRouteModeExact
)

// UseOption configures how module.Use registers routes for one or more phases.
type UseOption struct {
	mode   useRouteMode
	phases []apiaction.Phase
}

// CRUD registers phases using the framework's default CRUD route pattern
// (see crudRoute). A phase no HTTP route serves is refused by name, the way
// Exact refuses it.
func CRUD(phases ...apiaction.Phase) UseOption {
	return UseOption{
		mode:   useRouteModeCRUD,
		phases: servedPhases("CRUD", phases),
	}
}

// Exact registers phases against the module route exactly as declared. A
// phase no HTTP route serves is refused by name.
func Exact(phases ...apiaction.Phase) UseOption {
	return UseOption{
		mode:   useRouteModeExact,
		phases: servedPhases("Exact", phases),
	}
}

// servedPhases returns phases once every one of them is an action an HTTP
// route serves, panicking with the name of the first that is not, under the
// name of the declaring call: Stream is served over gRPC alone, and a hook
// phase is no action. A module declares its phases at registration, where
// a wrong one is a defect of the module to fail on, not a route to go
// missing without a word.
func servedPhases(call string, phases []apiaction.Phase) []apiaction.Phase {
	for _, p := range phases {
		if p.HTTPMethod() == "" {
			panic(fmt.Sprintf("module.%s: phase %s has no HTTP route", call, p))
		}
	}
	return phases
}

// Init releases pending module registrations.
//
// Module registration runs asynchronously because module.Use can be called before
// the framework router and service registries are initialized. Init only opens
// that gate; callers that need registered models, services, or routes to be
// visible must call Wait after Init.
func Init() error {
	if initialized.CompareAndSwap(false, true) {
		close(notify)
	}

	return nil
}

// Wait blocks until pending module.Use registrations are complete.
//
// It waits for model, service, and route registration only. Table creation
// is handled separately by the database runtime. Callers that need
// module-provided tables to exist must call the database runtime drain after
// Wait, not before it, because module registration can enqueue new
// table registrations.
func Wait() {
	registerMu.Lock()
	defer registerMu.Unlock()

	for pendingRegister != 0 {
		registerCond.Wait()
	}
}

// Use registers a module with the framework, automatically setting up model,
// service, and HTTP route registration for the specified route options.
//
// Type Parameters:
//   - M: Model type implementing types.Model
//   - REQ: Request type for API operations
//   - RSP: Response type for API operations
//
// Parameters:
//   - mod: Module instance implementing types.Module[M, REQ, RSP]
//   - options: Route options created by CRUD or Exact.
//
// Routes are registered based on mod.Route() and mod.Param().
// Authentication is determined by mod.Pub().
//
// Must be called during application initialization, typically in a Register() function.
func Use[M types.Model, REQ types.Request, RSP types.Response](mod types.Module[M, REQ, RSP], options ...UseOption) {
	startRegister(func() {
		<-notify

		if registersModel(options) {
			modelregistry.Register[M]()
		}

		// The route registers by the path it is served at, however the
		// module spells it; see apiaction.APIPath.
		route := apiaction.APIPath(mod.Route())

		param := mod.Param()
		param = strings.TrimFunc(param, func(r rune) bool {
			return r == ' ' || r == '{' || r == '}' || r == '[' || r == ']' || r == ':'
		})
		if len(param) == 0 {
			param = apiaction.ParamID
		}

		for _, option := range options {
			for _, p := range option.phases {
				// The service registers under the same path the router
				// handler is registered at, because the registry keys
				// services by path and phase.
				switch option.mode {
				case useRouteModeCRUD:
					serviceregistry.RegisterInstance[M, REQ, RSP](p, crudRoute(route, param, p), mod.Service())
					registerCRUDRouter(mod, route, param, p)
				case useRouteModeExact:
					serviceregistry.RegisterInstance[M, REQ, RSP](p, route, mod.Service())
					registerRouter(mod, route, nil, p)
				}
			}
		}
	})
}

func startRegister(fn func()) {
	registerMu.Lock()
	pendingRegister++
	registerMu.Unlock()

	go func() {
		defer finishRegister()
		fn()
	}()
}

func finishRegister() {
	registerMu.Lock()
	defer registerMu.Unlock()

	pendingRegister--
	if pendingRegister == 0 {
		registerCond.Broadcast()
	}
}

func registersModel(options []UseOption) bool {
	for _, option := range options {
		if option.mode == useRouteModeCRUD {
			return true
		}
	}
	return false
}

// crudRoute returns the raw route string of the phase under the CRUD layout,
// the layout gg gen registers a generated project's routes by (see
// apiaction.RouteSuffix). Service registration and router registration both
// derive the registry key from this route, so they must share this single
// mapping.
func crudRoute(route, param string, phase apiaction.Phase) string {
	if suffix := apiaction.RouteSuffix(phase, ":"+param); suffix != "" {
		return route + "/" + suffix
	}
	return route
}

// registerCRUDRouter registers the route of phase under the CRUD layout; an
// action on one row reads the row's id from the route's parameter.
func registerCRUDRouter[M types.Model, REQ types.Request, RSP types.Response](mod types.Module[M, REQ, RSP], route, param string, phase apiaction.Phase) {
	var cfg *types.ControllerConfig[M]
	if item := ":" + param; apiaction.RouteSuffix(phase, item) == item {
		cfg = &types.ControllerConfig[M]{ParamName: param}
	}
	registerRouter(mod, crudRoute(route, param, phase), cfg, phase)
}

// registerRouter registers an HTTP route with the appropriate router based on mod.Pub().
// If mod.Pub() returns true, registers with public router; otherwise with authenticated router.
func registerRouter[M types.Model, REQ types.Request, RSP types.Response](mod types.Module[M, REQ, RSP], route string, cfg *types.ControllerConfig[M], phase apiaction.Phase) {
	if mod.Pub() {
		// Register with public router - no authentication required
		router.Register[M, REQ, RSP](router.Pub(), route, cfg, phase)
	} else {
		// Register with authenticated router - authentication/authorization required
		router.Register[M, REQ, RSP](router.Auth(), route, cfg, phase)
	}
}
