package modelregistry

import (
	"reflect"
	"sync"

	"github.com/hydroan/gst/internal/hookoverride"
	"github.com/hydroan/gst/types/consts"
)

// Hook override detection for models.
//
// Every model carries the ten lifecycle hooks of types.Model through the
// embedded framework base, whose implementations are no-ops. Knowing which of
// them a model overrides lets the database layer drop work that exists only
// for the hooks' sake. A write wrapped in a transaction protects the atomicity
// of the hooks around the statement — but when neither hook of the pair is
// overridden there is nothing to keep atomic, and a single-statement write can
// run on the statement's own atomicity instead of paying a BEGIN/COMMIT pair.
// A hook wrapped in a span records where its time went — but a hook that is
// not overridden is the base no-op, and a span timing nothing only adds noise
// to the trace.
//
// The walk that resolves an override lives in hookoverride; this file says
// which types are the framework bases, which hooks are tracked, and caches
// the answers per model type.

// frameworkHookBases are the embeddable framework types whose hooks are the
// canonical no-ops. A hook whose declaration resolves to one of them is not
// an override. Keyed by the struct type, which is what hookoverride reports a
// declarer as.
var frameworkHookBases = map[reflect.Type]struct{}{
	reflect.TypeFor[Base]():     {},
	reflect.TypeFor[AutoBase](): {},
	reflect.TypeFor[Empty]():    {},
}

// isFrameworkBase reports whether typ is one of the framework bases.
func isFrameworkBase(typ reflect.Type) bool {
	_, ok := frameworkHookBases[typ]
	return ok
}

// Positions of the lifecycle hooks in hookPhases, hookMethodNames and
// hookOverrides, in the order types.Model declares them.
const (
	hookCreateBefore = iota
	hookCreateAfter
	hookDeleteBefore
	hookDeleteAfter
	hookUpdateBefore
	hookUpdateAfter
	hookListBefore
	hookListAfter
	hookGetBefore
	hookGetAfter
	hookCount
)

// hookPhases lists the phases of the lifecycle hooks of types.Model by
// position. It is the set the detection tracks; a phase outside it is not a
// model hook, and OverridesHook fails closed on it.
var hookPhases = [hookCount]consts.Phase{
	hookCreateBefore: consts.PHASE_CREATE_BEFORE,
	hookCreateAfter:  consts.PHASE_CREATE_AFTER,
	hookDeleteBefore: consts.PHASE_DELETE_BEFORE,
	hookDeleteAfter:  consts.PHASE_DELETE_AFTER,
	hookUpdateBefore: consts.PHASE_UPDATE_BEFORE,
	hookUpdateAfter:  consts.PHASE_UPDATE_AFTER,
	hookListBefore:   consts.PHASE_LIST_BEFORE,
	hookListAfter:    consts.PHASE_LIST_AFTER,
	hookGetBefore:    consts.PHASE_GET_BEFORE,
	hookGetAfter:     consts.PHASE_GET_AFTER,
}

// hookMethodNames holds the Go method name of each hook phase, derived once
// at initialization so the detection never repeats the case conversion.
var hookMethodNames = func() (names [hookCount]string) {
	for i, phase := range hookPhases {
		names[i] = phase.MethodName()
	}
	return names
}()

// hookIndex returns the position of phase in hookPhases, or false when the
// phase is not a model hook.
func hookIndex(phase consts.Phase) (int, bool) {
	for i, tracked := range hookPhases {
		if tracked == phase {
			return i, true
		}
	}
	return 0, false
}

// hookOverrides records, by position in hookPhases, which hooks a model type
// overrides.
type hookOverrides [hookCount]bool

// allHooksOverridden is the fail-closed answer for a value whose type cannot
// be inspected.
var allHooksOverridden = func() hookOverrides {
	var all hookOverrides
	for i := range all {
		all[i] = true
	}
	return all
}()

// hookOverridesCache memoizes the detection per model type; the method set of
// a type is fixed for the life of the binary.
var hookOverridesCache sync.Map

// OverridesCreateHooks reports whether the model overrides CreateBefore or
// CreateAfter beyond the framework base's no-op implementations.
func OverridesCreateHooks(m any) bool {
	overrides := hookOverridesOf(m)
	return overrides[hookCreateBefore] || overrides[hookCreateAfter]
}

// OverridesUpdateHooks reports whether the model overrides UpdateBefore or
// UpdateAfter beyond the framework base's no-op implementations.
func OverridesUpdateHooks(m any) bool {
	overrides := hookOverridesOf(m)
	return overrides[hookUpdateBefore] || overrides[hookUpdateAfter]
}

// OverridesDeleteHooks reports whether the model overrides DeleteBefore or
// DeleteAfter beyond the framework base's no-op implementations.
func OverridesDeleteHooks(m any) bool {
	overrides := hookOverridesOf(m)
	return overrides[hookDeleteBefore] || overrides[hookDeleteAfter]
}

// OverridesHook reports whether the model overrides the hook that phase names
// beyond the framework base's no-op implementation. Only the hook phases of
// types.Model are tracked; any other phase reports true, so a caller gating
// work on the answer keeps that work rather than skipping it for a hook the
// detection does not know.
func OverridesHook(m any, phase consts.Phase) bool {
	i, ok := hookIndex(phase)
	if !ok {
		return true
	}
	return hookOverridesOf(m)[i]
}

func hookOverridesOf(m any) hookOverrides {
	typ := reflect.TypeOf(m)
	if typ == nil {
		// No type resolves no methods; fail closed.
		return allHooksOverridden
	}
	if cached, ok := hookOverridesCache.Load(typ); ok {
		return cached.(hookOverrides) //nolint:errcheck
	}

	var overrides hookOverrides
	for i, name := range hookMethodNames {
		overrides[i] = hookoverride.Overridden(typ, name, isFrameworkBase)
	}
	hookOverridesCache.Store(typ, overrides)
	return overrides
}
