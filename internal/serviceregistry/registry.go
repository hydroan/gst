package serviceregistry

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"

	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/types"
)

var (
	mu       sync.RWMutex
	services = make(map[string]any)
)

var errNotFoundService = errors.New("no service instance matches the given route and phase, skip processing service layer")

// RegisterInstance registers a concrete service instance for the route and
// phase.
//
// The route names the one the HTTP layer registers the matching handler
// under, spelled either way consts.APIPath accepts, because Key derives the
// registry key from the path the route is served at and the controller
// handlers resolve services through that key. An empty route panics.
//
// Registering a second service under one route and phase panics: a silent
// overwrite would dispatch requests to the wrong service, which is exactly
// the failure mode the route-derived key exists to prevent.
//
// RegisterInstance preserves any preconfigured fields on svc. If svc is a
// non-pointer value, the registry stores a pointer copy so controller lookups
// always work with pointer service instances.
func RegisterInstance[M types.Model, REQ types.Request, RSP types.Response](phase consts.Phase, route string, svc types.Service[M, REQ, RSP]) {
	if svc == nil {
		return
	}
	route = strings.TrimSpace(route)
	if len(route) == 0 {
		panic("serviceregistry: register requires a non-empty route")
	}

	val := reflect.ValueOf(svc)
	if !val.IsValid() {
		return
	}
	var stored any = svc
	if val.Kind() == reflect.Pointer && val.IsNil() {
		stored = reflect.New(val.Type().Elem()).Interface()
	} else if val.Kind() != reflect.Pointer {
		ptr := reflect.New(val.Type())
		ptr.Elem().Set(val)
		stored = ptr.Interface()
	}

	mu.Lock()
	defer mu.Unlock()

	key := Key(phase, route)
	if existing, ok := services[key]; ok {
		panic(fmt.Sprintf("serviceregistry: duplicate service registration for route %q phase %q: %T is already registered, cannot register %T", route, phase, existing, stored))
	}

	setLogger(stored)
	services[key] = stored
}

// Register registers the service type S for the route and phase, the
// registration service.Register forwards to: S is normally a pointer to a
// struct type embedding Base, and a fresh pointer instance of it is stored.
// A pointer to any concrete type that satisfies the constraint implements the
// service interface, so only an interface type argument fails, with a panic:
// it names no service to dispatch to, and registering nothing in its place
// would leave the route on the built-in handling without a word.
func Register[S types.Service[M, REQ, RSP], M types.Model, REQ types.Request, RSP types.Response](phase consts.Phase, route string) {
	typ := reflect.TypeFor[S]()
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	svc, ok := reflect.TypeAssert[types.Service[M, REQ, RSP]](reflect.New(typ))
	if !ok {
		panic(fmt.Sprintf("service: register of route %q phase %q requires a concrete service type, not the interface %s", route, phase, typ))
	}
	RegisterInstance[M, REQ, RSP](phase, route, svc)
}

// Key returns the registry key of the route and phase: the path the route
// is served at (see consts.APIPath) and the phase, so registration and
// resolution agree however either side spells the route, with or without
// the prefix or stray whitespace. A blank route, which RegisterInstance refuses,
// keys no service, not even one registered on the root of the prefix: the
// key of a handler built without a route resolves none.
//
// The key deliberately carries no type information: Go type aliases collapse
// distinct request/response declarations into one type, so a type-derived key
// cannot tell two actions apart. The route is unique per action by HTTP
// routing rules, which makes route plus phase a collision-free identity.
func Key(phase consts.Phase, route string) string {
	if route = strings.TrimSpace(route); route != "" {
		route = consts.APIPath(route)
	}
	return route + "|" + string(phase)
}
