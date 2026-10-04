package serviceregistry

import (
	"reflect"
	"strings"
	"sync"

	"github.com/hydroan/gst/internal/hookoverride"
	"github.com/hydroan/gst/internal/modelregistry"
)

// Hook override detection for services.
//
// Every service carries its hooks through the embedded Base, whose
// implementations are no-ops. The controller wraps each hook it invokes in a
// span; a hook the service does not override is Base's no-op, and a span
// timing nothing only adds noise to the trace, so the controller asks here
// before starting one. The walk that resolves an override lives in
// hookoverride; this file says which types are the base, which methods are
// tracked, and caches the answers per service type.

// baseType is one instantiation of Base. Its package path and name prefix
// identify every other instantiation, and its method set is the set of hooks
// a service may override, read off the type so the tracked set cannot drift
// from it.
var baseType = reflect.TypeFor[Base[*modelregistry.Empty, any, any]]()

// isFrameworkBase reports whether typ is an instantiation of Base.
func isFrameworkBase(typ reflect.Type) bool {
	return typ.PkgPath() == baseType.PkgPath() && strings.HasPrefix(typ.Name(), "Base[")
}

// hookMethodNames lists the methods Base itself declares. The methods its
// embedded logger promotes resolve to no declaring struct at all, which the
// walk reports as overridden, and that is what leaves them out: they are not
// hooks a service overrides.
var hookMethodNames = func() []string {
	typ := reflect.PointerTo(baseType)
	names := make([]string, 0, typ.NumMethod())
	for method := range typ.Methods() {
		if !hookoverride.Overridden(typ, method.Name, isFrameworkBase) {
			names = append(names, method.Name)
		}
	}
	return names
}()

// hookOverrides records, per method Base declares, whether a service type
// overrides it.
type hookOverrides map[string]bool

// hookOverridesCache memoizes the detection per service type; the method set
// of a type is fixed for the life of the binary.
var hookOverridesCache sync.Map

// OverridesHook reports whether svc overrides the named hook beyond Base's
// no-op implementation. Only the methods Base declares are tracked; any other
// name, and a nil service, report true, so a caller gating work on the answer
// keeps that work rather than skipping it for a hook the detection does not
// know.
func OverridesHook(svc any, method string) bool {
	typ := reflect.TypeOf(svc)
	if typ == nil {
		return true
	}
	overridden, tracked := hookOverridesOf(typ)[method]
	if !tracked {
		return true
	}
	return overridden
}

func hookOverridesOf(typ reflect.Type) hookOverrides {
	if cached, ok := hookOverridesCache.Load(typ); ok {
		return cached.(hookOverrides) //nolint:errcheck
	}

	overrides := make(hookOverrides, len(hookMethodNames))
	for _, name := range hookMethodNames {
		overrides[name] = hookoverride.Overridden(typ, name, isFrameworkBase)
	}
	hookOverridesCache.Store(typ, overrides)
	return overrides
}
