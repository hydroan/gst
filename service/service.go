// Package service exposes the public service extension points for application code.
//
// Application services should embed Base and register service types with Register.
// Controller lookup, concrete instance registration, and registry state are kept
// inside the framework so external projects cannot mutate framework-owned service
// mappings directly.
package service

import (
	"fmt"
	"reflect"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/internal/types"
)

// Base is the default no-op service implementation exposed to application services.
type Base[M types.Model, REQ types.Request, RSP types.Response] = serviceregistry.Base[M, REQ, RSP]

// Register registers a service type for the phase of the route.
//
// Register is the public service registration entry point for application and
// generated service code. Framework internals handle service lookup and concrete
// instance registration through an internal registry.
//
// The route must be the same raw route string the matching router.Register
// call uses, because the registry keys services by route and phase. Generated
// code guarantees the match by deriving both registrations from one design;
// keep hand-written registrations aligned the same way. Hook services (whose
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
//	    service.Register[*myService](consts.PHASE_CREATE, "samples")
//	}
//
// Logger initialization:
//   - If Register is called in an "init" function, logger.Service may be nil,
//     and the service.Logger will be set later, while the framework bootstraps.
//   - If Register is called after initialization (e.g., in Init function),
//     logger.Service is already available, and the service.Logger will be set directly.
func Register[S types.Service[M, REQ, RSP], M types.Model, REQ types.Request, RSP types.Response](phase consts.Phase, route string) {
	typ := reflect.TypeFor[S]()
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	// A pointer to any concrete type that satisfies the constraint implements
	// the service interface, so only an interface type argument fails here:
	// it names no service to dispatch to, and registering nothing in its place
	// would leave the route on the built-in handling without a word.
	svc, ok := reflect.TypeAssert[types.Service[M, REQ, RSP]](reflect.New(typ))
	if !ok {
		panic(fmt.Sprintf("service: register of route %q phase %q requires a concrete service type, not the interface %s", route, phase, typ))
	}
	serviceregistry.Register[M, REQ, RSP](phase, route, svc)
}
