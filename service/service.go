// Package service exposes the public service extension points for
// application code: Base, the no-op service an application service embeds,
// Register, which registers a service type for a phase of a route, and the
// phases of an action a registration names, Create or List.
//
// Controller lookup, concrete instance registration, and registry state are
// kept inside the framework so external projects cannot mutate
// framework-owned service mappings directly.
package service

import (
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/internal/types"
)

// Base is the default no-op service implementation exposed to application services.
type Base[M types.Model, REQ types.Request, RSP types.Response] = serviceregistry.Base[M, REQ, RSP]

// Phase is a phase of an action: the action itself, Create and its kind,
// Import, Export, SSE and Stream, and, for the CRUD and batch actions, the
// hooks run before and after it, CreateBefore and CreateAfter. The value is
// the snake case name the service registry keys by and the logs carry; the
// name of a phase is the identifier its constant is declared with, which
// is the DSL keyword declaring the action and the service method serving it.
type Phase = apiaction.Phase

// The phases of an action. Generated code refers to a phase by its name.
const (
	Create = apiaction.Create
	Delete = apiaction.Delete
	Update = apiaction.Update
	Patch  = apiaction.Patch
	List   = apiaction.List
	Get    = apiaction.Get

	CreateMany = apiaction.CreateMany
	DeleteMany = apiaction.DeleteMany
	UpdateMany = apiaction.UpdateMany
	PatchMany  = apiaction.PatchMany

	CreateBefore = apiaction.CreateBefore
	CreateAfter  = apiaction.CreateAfter
	DeleteBefore = apiaction.DeleteBefore
	DeleteAfter  = apiaction.DeleteAfter
	UpdateBefore = apiaction.UpdateBefore
	UpdateAfter  = apiaction.UpdateAfter
	PatchBefore  = apiaction.PatchBefore
	PatchAfter   = apiaction.PatchAfter
	ListBefore   = apiaction.ListBefore
	ListAfter    = apiaction.ListAfter
	GetBefore    = apiaction.GetBefore
	GetAfter     = apiaction.GetAfter

	CreateManyBefore = apiaction.CreateManyBefore
	CreateManyAfter  = apiaction.CreateManyAfter
	DeleteManyBefore = apiaction.DeleteManyBefore
	DeleteManyAfter  = apiaction.DeleteManyAfter
	UpdateManyBefore = apiaction.UpdateManyBefore
	UpdateManyAfter  = apiaction.UpdateManyAfter
	PatchManyBefore  = apiaction.PatchManyBefore
	PatchManyAfter   = apiaction.PatchManyAfter

	Import = apiaction.Import
	Export = apiaction.Export

	SSE = apiaction.SSE

	Stream = apiaction.Stream
)

// Register registers a service type for the phase of the route.
//
// Register is the public service registration entry point for application and
// generated service code. Framework internals handle service lookup and concrete
// instance registration through an internal registry.
//
// The route names the one the matching router.Register call registers,
// spelled either way router.APIPath accepts, /api/samples or samples,
// because the registry keys services by the path the route is served at
// and the phase. Generated code writes the path and guarantees the match by
// deriving both registrations from one design; keep hand-written
// registrations aligned the same way. Hook services (whose
// model, request, and response types are identical) register with their route
// all the same. Registering two services under one route and phase panics at
// startup instead of silently overwriting the first one, and so does an
// interface type argument, which names no service to register.
//
// The service type parameter S is normally a pointer to a struct type
// embedding Base; the registry always stores a pointer instance.
//
// Example usage:
//
//	type myService struct {
//	    service.Base[*model.Sample, *model.SampleCreateReq, *model.SampleCreateRsp]
//	}
//
//	func init() {
//	    service.Register[*myService](service.Create, "/api/samples")
//	}
//
// Logger initialization:
//   - If Register is called in an "init" function, logger.Service may be nil,
//     and the service.Logger will be set later, while the framework bootstraps.
//   - If Register is called after initialization (e.g., in Init function),
//     logger.Service is already available, and the service.Logger will be set directly.
func Register[S types.Service[M, REQ, RSP], M types.Model, REQ types.Request, RSP types.Response](phase apiaction.Phase, route string) {
	serviceregistry.Register[S, M, REQ, RSP](phase, route)
}
