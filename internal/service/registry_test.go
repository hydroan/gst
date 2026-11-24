package service_test

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/logger"
	"github.com/hydroan/gst/internal/model"
	"github.com/hydroan/gst/internal/service"
	"github.com/stretchr/testify/require"
)

// testRecord is a second fixture model so type-mismatch tests can resolve a
// registered service with different type parameters.
type testRecord struct {
	Name string
	model.Base
}

func TestRegisterAndResolve(t *testing.T) {
	logger.Service = logger.Fallback("service")

	type svc struct {
		service.Base[*testUser, *testUser, *testUser]
	}

	registered := &svc{}
	phase := newPhase("test_register_and_resolve")

	service.RegisterInstance[*testUser, *testUser, *testUser](phase, "samples", registered)
	resolved := service.Resolve[*testUser, *testUser, *testUser](service.Key(phase, "samples"))

	require.Same(t, registered, resolved)
	require.NotNil(t, registered.Logger)
}

// TestKeyOfABlankRouteResolvesNoService pins what the controller relies on
// for a handler built without a route: its key names no service, not even
// one registered on the root of the API prefix, the path a blank route
// would otherwise be served at.
func TestKeyOfABlankRouteResolvesNoService(t *testing.T) {
	type svc struct {
		service.Base[*testUser, *testUser, *testUser]
	}

	phase := newPhase("test_blank_route_key")
	service.RegisterInstance[*testUser, *testUser, *testUser](phase, "/", &svc{})
	resolved := service.Resolve[*testUser, *testUser, *testUser](service.Key(phase, ""))

	_, ok := resolved.(*service.Base[*testUser, *testUser, *testUser])
	require.True(t, ok)
}

func TestResolveReturnsBaseWhenServiceMissing(t *testing.T) {
	key := service.Key(apiaction.Phase("test_missing_service"), "samples")
	resolved := service.Resolve[*testUser, *testUser, *testUser](key)

	_, ok := resolved.(*service.Base[*testUser, *testUser, *testUser])
	require.True(t, ok)
}

// TestResolveSeesLateRegistration guards the contract the controller handlers
// rely on: the key may be built before the service is registered, and
// per-request resolution through that key must still find the service.
func TestResolveSeesLateRegistration(t *testing.T) {
	type svc struct {
		service.Base[*testUser, *testUser, *testUser]
	}

	phase := newPhase("test_resolve_late_registration")
	key := service.Key(phase, "samples")

	resolved := service.Resolve[*testUser, *testUser, *testUser](key)
	_, ok := resolved.(*service.Base[*testUser, *testUser, *testUser])
	require.True(t, ok, "missing service should resolve to the no-op Base")

	registered := &svc{}
	service.RegisterInstance[*testUser, *testUser, *testUser](phase, "samples", registered)

	require.Same(t, registered, service.Resolve[*testUser, *testUser, *testUser](key))
}

// TestRegisterKeysByRoute guards against silent overwrites: two services
// sharing one model/request/response type tuple (as type aliases collapse
// distinct declarations into one type) must dispatch independently when they
// are registered under different routes.
func TestRegisterKeysByRoute(t *testing.T) {
	type startSvc struct {
		service.Base[*testUser, *testUser, *testUser]
	}
	type stopSvc struct {
		service.Base[*testUser, *testUser, *testUser]
	}

	phase := newPhase("test_register_keys_by_route")
	start := &startSvc{}
	stop := &stopSvc{}

	service.RegisterInstance[*testUser, *testUser, *testUser](phase, "samples/:id/start", start)
	service.RegisterInstance[*testUser, *testUser, *testUser](phase, "samples/:id/stop", stop)

	require.Same(t, start, service.Resolve[*testUser, *testUser, *testUser](service.Key(phase, "samples/:id/start")))
	require.Same(t, stop, service.Resolve[*testUser, *testUser, *testUser](service.Key(phase, "samples/:id/stop")))
}

// TestRegisterPanicsOnDuplicateRouteAndPhase pins the fail-fast contract: a
// second registration under one route and phase must panic at startup instead
// of silently overwriting the first service.
func TestRegisterPanicsOnDuplicateRouteAndPhase(t *testing.T) {
	type svc struct {
		service.Base[*testUser, *testUser, *testUser]
	}

	phase := newPhase("test_register_duplicate")
	service.RegisterInstance[*testUser, *testUser, *testUser](phase, "samples", &svc{})

	require.Panics(t, func() {
		service.RegisterInstance[*testUser, *testUser, *testUser](phase, "samples", &svc{})
	})
}

func TestRegisterPanicsOnEmptyRoute(t *testing.T) {
	type svc struct {
		service.Base[*testUser, *testUser, *testUser]
	}

	require.Panics(t, func() {
		service.RegisterInstance[*testUser, *testUser, *testUser](apiaction.Phase("test_register_empty_route"), "  ", &svc{})
	})
}

// TestResolveReturnsBaseOnTypeMismatch covers the wiring-bug path opened by
// the type-free key: a hand-written registration can disagree with the
// resolving handler's type parameters, and the mismatch must degrade to the
// no-op Base instead of panicking mid-request.
func TestResolveReturnsBaseOnTypeMismatch(t *testing.T) {
	type svc struct {
		service.Base[*testUser, *testUser, *testUser]
	}

	phase := newPhase("test_resolve_type_mismatch")
	service.RegisterInstance[*testUser, *testUser, *testUser](phase, "samples", &svc{})

	var resolved any
	require.NotPanics(t, func() {
		resolved = service.Resolve[*testRecord, *testRecord, *testRecord](service.Key(phase, "samples"))
	})
	_, ok := resolved.(*service.Base[*testRecord, *testRecord, *testRecord])
	require.True(t, ok, "type mismatch should degrade to the no-op Base")
}

// phaseSeq numbers the phases the tests register under. The registry refuses
// a second registration of one route and phase and lives as long as the
// process, so a repeated run (go test -count) registers under phases of its
// own.
var phaseSeq atomic.Int64

// newPhase returns a phase named after name that no earlier registration of
// the process used.
func newPhase(name string) apiaction.Phase {
	return apiaction.Phase(fmt.Sprintf("%s-%d", name, phaseSeq.Add(1)))
}
