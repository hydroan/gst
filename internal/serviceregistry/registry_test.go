package serviceregistry_test

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/logger/zap"
	"github.com/stretchr/testify/require"
)

// testRecord is a second fixture model so type-mismatch tests can resolve a
// registered service with different type parameters.
type testRecord struct {
	Name string
	modelregistry.Base
}

func TestRegisterAndResolve(t *testing.T) {
	logger.Service = zap.Fallback("service")

	type svc struct {
		serviceregistry.Base[*testUser, *testUser, *testUser]
	}

	registered := &svc{}
	phase := newPhase("test_register_and_resolve")

	serviceregistry.Register[*testUser, *testUser, *testUser](phase, "samples", registered)
	resolved := serviceregistry.Resolve[*testUser, *testUser, *testUser](serviceregistry.Key(phase, "samples"))

	require.Same(t, registered, resolved)
	require.NotNil(t, registered.Logger)
}

func TestResolveReturnsBaseWhenServiceMissing(t *testing.T) {
	key := serviceregistry.Key(consts.Phase("test_missing_service"), "samples")
	resolved := serviceregistry.Resolve[*testUser, *testUser, *testUser](key)

	_, ok := resolved.(*serviceregistry.Base[*testUser, *testUser, *testUser])
	require.True(t, ok)
}

// TestResolveSeesLateRegistration guards the contract controller factories
// rely on: the key may be built before the service is registered, and
// per-request resolution through that key must still find the service.
func TestResolveSeesLateRegistration(t *testing.T) {
	type svc struct {
		serviceregistry.Base[*testUser, *testUser, *testUser]
	}

	phase := newPhase("test_resolve_late_registration")
	key := serviceregistry.Key(phase, "samples")

	resolved := serviceregistry.Resolve[*testUser, *testUser, *testUser](key)
	_, ok := resolved.(*serviceregistry.Base[*testUser, *testUser, *testUser])
	require.True(t, ok, "missing service should resolve to the no-op Base")

	registered := &svc{}
	serviceregistry.Register[*testUser, *testUser, *testUser](phase, "samples", registered)

	require.Same(t, registered, serviceregistry.Resolve[*testUser, *testUser, *testUser](key))
}

// TestRegisterKeysByRoute guards the fix for silent overwrites: two services
// sharing one model/request/response type tuple (as type aliases collapse
// distinct declarations into one type) must dispatch independently when they
// are registered under different routes.
func TestRegisterKeysByRoute(t *testing.T) {
	type startSvc struct {
		serviceregistry.Base[*testUser, *testUser, *testUser]
	}
	type stopSvc struct {
		serviceregistry.Base[*testUser, *testUser, *testUser]
	}

	phase := newPhase("test_register_keys_by_route")
	start := &startSvc{}
	stop := &stopSvc{}

	serviceregistry.Register[*testUser, *testUser, *testUser](phase, "samples/:id/start", start)
	serviceregistry.Register[*testUser, *testUser, *testUser](phase, "samples/:id/stop", stop)

	require.Same(t, start, serviceregistry.Resolve[*testUser, *testUser, *testUser](serviceregistry.Key(phase, "samples/:id/start")))
	require.Same(t, stop, serviceregistry.Resolve[*testUser, *testUser, *testUser](serviceregistry.Key(phase, "samples/:id/stop")))
}

// TestRegisterPanicsOnDuplicateRouteAndPhase pins the fail-fast contract: a
// second registration under one route and phase must panic at startup instead
// of silently overwriting the first service.
func TestRegisterPanicsOnDuplicateRouteAndPhase(t *testing.T) {
	type svc struct {
		serviceregistry.Base[*testUser, *testUser, *testUser]
	}

	phase := newPhase("test_register_duplicate")
	serviceregistry.Register[*testUser, *testUser, *testUser](phase, "samples", &svc{})

	require.Panics(t, func() {
		serviceregistry.Register[*testUser, *testUser, *testUser](phase, "samples", &svc{})
	})
}

func TestRegisterPanicsOnEmptyRoute(t *testing.T) {
	type svc struct {
		serviceregistry.Base[*testUser, *testUser, *testUser]
	}

	require.Panics(t, func() {
		serviceregistry.Register[*testUser, *testUser, *testUser](consts.Phase("test_register_empty_route"), "  ", &svc{})
	})
}

// TestResolveReturnsBaseOnTypeMismatch covers the wiring-bug path opened by
// the type-free key: a hand-written registration can disagree with the
// resolving factory's type parameters, and the mismatch must degrade to the
// no-op Base instead of panicking mid-request.
func TestResolveReturnsBaseOnTypeMismatch(t *testing.T) {
	type svc struct {
		serviceregistry.Base[*testUser, *testUser, *testUser]
	}

	phase := newPhase("test_resolve_type_mismatch")
	serviceregistry.Register[*testUser, *testUser, *testUser](phase, "samples", &svc{})

	var resolved any
	require.NotPanics(t, func() {
		resolved = serviceregistry.Resolve[*testRecord, *testRecord, *testRecord](serviceregistry.Key(phase, "samples"))
	})
	_, ok := resolved.(*serviceregistry.Base[*testRecord, *testRecord, *testRecord])
	require.True(t, ok, "type mismatch should degrade to the no-op Base")
}

// phaseSeq numbers the phases the tests register under. The registry refuses
// a second registration of one route and phase and lives as long as the
// process, so a repeated run (go test -count) registers under phases of its
// own.
var phaseSeq atomic.Int64

// newPhase returns a phase named after name that no earlier registration of
// the process used.
func newPhase(name string) consts.Phase {
	return consts.Phase(fmt.Sprintf("%s-%d", name, phaseSeq.Add(1)))
}
