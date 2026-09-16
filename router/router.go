// Package router registers a project's HTTP routes and the hooks that run
// once they are ready. The generated route registrations call Register on the
// Auth or Pub group, a project seeds its data from OnRoutesReady, and Routes
// lists what was registered. The server itself — building the engine,
// serving, shutting down — is the framework's to run.
//
// This package forwards to the framework-internal implementation and adds no
// behavior of its own.
package router

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/consts"
	internalrouter "github.com/hydroan/gst/internal/router"
	"github.com/hydroan/gst/internal/types"
)

// Register registers route on router for each of verbs. router is the route
// group the route belongs to: Auth, whose routes run the middleware
// registered with middleware.RegisterAuth, or Pub, whose routes do not. Each
// verb is served by the framework's handler for it, under its HTTP method:
//
//   - POST: Create, CreateMany, Import
//   - DELETE: Delete, DeleteMany
//   - PUT: Update, UpdateMany
//   - PATCH: Patch, PatchMany
//   - GET: List, Get, Export, SSE
//
// The route is registered as written, under the API prefix: "records/:rec"
// serves /api/records/:rec, and a route that already starts with "/api/" is
// not prefixed twice. It must equal the route of the matching
// service.Register call, because a handler finds its service by route and
// phase; generated code derives both from one design. cfg may be nil, and it
// is copied, so one config can serve several routes. A blank route, or no
// verbs, panics: the mistake stops the start instead of leaving an endpoint
// that answers 404.
func Register[M types.Model, REQ types.Request, RSP types.Response](router *gin.RouterGroup, route string, cfg *types.ControllerConfig[M], verbs ...consts.HTTPVerb) {
	internalrouter.Register[M, REQ, RSP](router, route, cfg, verbs...)
}

// Auth returns the route group, under the API prefix, whose routes run the
// middleware registered with middleware.RegisterAuth. It is nil until the
// framework has bootstrapped.
func Auth() *gin.RouterGroup {
	return internalrouter.Auth()
}

// Pub returns the route group, under the API prefix, for public routes: the
// middleware registered with middleware.RegisterAuth does not run on them.
// It is nil until the framework has bootstrapped.
func Pub() *gin.RouterGroup {
	return internalrouter.Pub()
}

// OnRoutesReady registers a hook that runs after all routes are registered
// and every table exists, before the components that run alongside the
// server start and before the server starts — the place for seeding the
// data the first round of a job and the first request both count on. Across
// a deployment the hooks run one process at a time, so a hook that reads
// before it writes never races another replica's; the others wait for as
// long as the hooks take, and the orchestrator's startup probe is what
// bounds a process stuck in them. A hook is therefore for the database:
// work that reaches other systems — a client to connect, a topic to
// create — holds every other replica's start for as long as that system
// takes to answer, and belongs elsewhere or behind a deadline of its own;
// work that runs for the life of the process — a consumer loop — is a
// component, see component.Register.
//
// The hook runs on the context of the start: the seeding's statements and
// transactions run on it, and a termination signal during the start ends
// it, so a hook cut short returns and the process stops cleanly. The
// context ends once the hooks have run, so nothing started on it outlives
// them. The hook also receives a route snapshot; mutating it does not
// change the router registry.
func OnRoutesReady(fn func(ctx context.Context, routes map[string][]string) error) {
	internalrouter.OnRoutesReady(fn)
}

// Routes returns a read-only snapshot of registered business API routes.
//
// Route parameters are converted from Gin's ":id" format to "{id}" so the
// returned paths can be reused as authorization policy objects and menu route
// bindings; "{id}" is the placeholder the RBAC matcher reads as one path
// segment. Mutating the returned map or method slices does not affect router
// state.
func Routes() map[string][]string {
	return internalrouter.Routes()
}
