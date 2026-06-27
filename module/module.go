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
//	        consts.PHASE_CREATE,
//	        consts.PHASE_LIST,
//	        consts.PHASE_GET,
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

	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/router"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
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
	phases []consts.Phase
}

// CRUD registers phases using the framework's default CRUD route pattern.
func CRUD(phases ...consts.Phase) UseOption {
	return UseOption{
		mode:   useRouteModeCRUD,
		phases: phases,
	}
}

// Exact registers phases against the module route exactly as declared.
func Exact(phases ...consts.Phase) UseOption {
	return UseOption{
		mode:   useRouteModeExact,
		phases: phases,
	}
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
// It waits for model, service, and route registration only. Database table
// creation and seed record insertion are handled separately by the database runtime.
// Callers that need module-provided tables and seed records to exist must call
// the database runtime drain after Wait, not before it, because module registration
// can enqueue new model.Register work.
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
			model.Register[M]()
		}

		route := mod.Route()
		route = strings.TrimPrefix(route, "/")
		route = strings.TrimPrefix(route, "api/")
		route = strings.TrimPrefix(route, "/")

		param := mod.Param()
		param = strings.TrimFunc(param, func(r rune) bool {
			return r == ' ' || r == '{' || r == '}' || r == '[' || r == ']' || r == ':'
		})
		if len(param) == 0 {
			param = "id"
		}

		for _, option := range options {
			for _, p := range option.phases {
				serviceregistry.Register[M, REQ, RSP](p, mod.Service())

				switch option.mode {
				case useRouteModeCRUD:
					registerCRUDRouter(mod, route, param, p)
				case useRouteModeExact:
					registerRouter(mod, route, nil, p.ToHTTPVerb())
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

func registerCRUDRouter[M types.Model, REQ types.Request, RSP types.Response](mod types.Module[M, REQ, RSP], route, param string, phase consts.Phase) {
	switch phase {
	case consts.PHASE_CREATE:
		registerRouter(mod, route, nil, consts.Create)
	case consts.PHASE_DELETE:
		registerRouter(mod, fmt.Sprintf("%s/:%s", route, param), &types.ControllerConfig[M]{ParamName: param}, consts.Delete)
	case consts.PHASE_UPDATE:
		registerRouter(mod, fmt.Sprintf("%s/:%s", route, param), &types.ControllerConfig[M]{ParamName: param}, consts.Update)
	case consts.PHASE_PATCH:
		registerRouter(mod, fmt.Sprintf("%s/:%s", route, param), &types.ControllerConfig[M]{ParamName: param}, consts.Patch)
	case consts.PHASE_LIST:
		registerRouter(mod, route, nil, consts.List)
	case consts.PHASE_GET:
		registerRouter(mod, fmt.Sprintf("%s/:%s", route, param), &types.ControllerConfig[M]{ParamName: param}, consts.Get)
	case consts.PHASE_CREATE_MANY:
		registerRouter(mod, route+"/batch", nil, consts.CreateMany)
	case consts.PHASE_DELETE_MANY:
		registerRouter(mod, route+"/batch", nil, consts.DeleteMany)
	case consts.PHASE_UPDATE_MANY:
		registerRouter(mod, route+"/batch", nil, consts.UpdateMany)
	case consts.PHASE_PATCH_MANY:
		registerRouter(mod, route+"/batch", nil, consts.PatchMany)
	}
}

// registerRouter registers an HTTP route with the appropriate router based on mod.Pub().
// If mod.Pub() returns true, registers with public router; otherwise with authenticated router.
func registerRouter[M types.Model, REQ types.Request, RSP types.Response](mod types.Module[M, REQ, RSP], route string, cfg *types.ControllerConfig[M], verb consts.HTTPVerb) {
	if mod.Pub() {
		// Register with public router - no authentication required
		router.Register[M, REQ, RSP](router.Pub(), route, cfg, verb)
	} else {
		// Register with authenticated router - authentication/authorization required
		router.Register[M, REQ, RSP](router.Auth(), route, cfg, verb)
	}
}
