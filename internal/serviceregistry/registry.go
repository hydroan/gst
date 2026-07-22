package serviceregistry

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"

	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

var (
	mu       sync.RWMutex
	services = make(map[string]any)
)

var errNotFoundService = errors.New("no service instance matches the given route and phase, skip processing service layer")

// Register registers a concrete service instance for the route and phase.
//
// The route must be the exact raw route string the HTTP layer registers the
// matching handler under, because Key derives the registry key from it and
// controller factories resolve services through that key. An empty route
// panics.
//
// Registering a second service under one route and phase panics: a silent
// overwrite would dispatch requests to the wrong service, which is exactly
// the failure mode the route-derived key exists to prevent.
//
// Register preserves any preconfigured fields on svc. If svc is a non-pointer
// value, the registry stores a pointer copy so controller lookups always work
// with pointer service instances.
func Register[M types.Model, REQ types.Request, RSP types.Response](phase consts.Phase, route string, svc types.Service[M, REQ, RSP]) {
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

// Key returns the registry key of the route and phase. The route is trimmed
// so registration and resolution agree even when one side carries stray
// whitespace.
//
// The key deliberately carries no type information: Go type aliases collapse
// distinct request/response declarations into one type, so a type-derived key
// cannot tell two actions apart. The route is unique per action by HTTP
// routing rules, which makes route plus phase a collision-free identity.
func Key(phase consts.Phase, route string) string {
	return strings.TrimSpace(route) + "|" + string(phase)
}
