package serviceregistry

import (
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/logger/zap"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
)

// testRecord is a second fixture model so type-mismatch tests can resolve a
// registered service with different type parameters.
type testRecord struct {
	Name string
	modelregistry.Base
}

func TestRegisterAndResolve(t *testing.T) {
	logger.Service = zap.New("")

	type svc struct {
		Base[*testUser, *testUser, *testUser]
	}

	registered := &svc{}
	phase := consts.Phase("test_register_and_resolve")

	Register[*testUser, *testUser, *testUser](phase, "samples", registered)
	resolved := Resolve[*testUser, *testUser, *testUser](Key(phase, "samples"))

	require.Same(t, registered, resolved)
	require.NotNil(t, registered.Logger)
}

func TestResolveReturnsBaseWhenServiceMissing(t *testing.T) {
	key := Key(consts.Phase("test_missing_service"), "samples")
	resolved := Resolve[*testUser, *testUser, *testUser](key)

	_, ok := resolved.(*Base[*testUser, *testUser, *testUser])
	require.True(t, ok)
}

// TestResolveSeesLateRegistration guards the contract controller factories
// rely on: the key may be built before the service is registered, and
// per-request resolution through that key must still find the service.
func TestResolveSeesLateRegistration(t *testing.T) {
	type svc struct {
		Base[*testUser, *testUser, *testUser]
	}

	phase := consts.Phase("test_resolve_late_registration")
	key := Key(phase, "samples")

	resolved := Resolve[*testUser, *testUser, *testUser](key)
	_, ok := resolved.(*Base[*testUser, *testUser, *testUser])
	require.True(t, ok, "missing service should resolve to the no-op Base")

	registered := &svc{}
	Register[*testUser, *testUser, *testUser](phase, "samples", registered)

	require.Same(t, registered, Resolve[*testUser, *testUser, *testUser](key))
}

// TestRegisterKeysByRoute guards the fix for silent overwrites: two services
// sharing one model/request/response type tuple (as type aliases collapse
// distinct declarations into one type) must dispatch independently when they
// are registered under different routes.
func TestRegisterKeysByRoute(t *testing.T) {
	type startSvc struct {
		Base[*testUser, *testUser, *testUser]
	}
	type stopSvc struct {
		Base[*testUser, *testUser, *testUser]
	}

	phase := consts.Phase("test_register_keys_by_route")
	start := &startSvc{}
	stop := &stopSvc{}

	Register[*testUser, *testUser, *testUser](phase, "samples/:id/start", start)
	Register[*testUser, *testUser, *testUser](phase, "samples/:id/stop", stop)

	require.Same(t, start, Resolve[*testUser, *testUser, *testUser](Key(phase, "samples/:id/start")))
	require.Same(t, stop, Resolve[*testUser, *testUser, *testUser](Key(phase, "samples/:id/stop")))
}

// TestRegisterPanicsOnDuplicateRouteAndPhase pins the fail-fast contract: a
// second registration under one route and phase must panic at startup instead
// of silently overwriting the first service.
func TestRegisterPanicsOnDuplicateRouteAndPhase(t *testing.T) {
	type svc struct {
		Base[*testUser, *testUser, *testUser]
	}

	phase := consts.Phase("test_register_duplicate")
	Register[*testUser, *testUser, *testUser](phase, "samples", &svc{})

	require.Panics(t, func() {
		Register[*testUser, *testUser, *testUser](phase, "samples", &svc{})
	})
}

func TestRegisterPanicsOnEmptyRoute(t *testing.T) {
	type svc struct {
		Base[*testUser, *testUser, *testUser]
	}

	require.Panics(t, func() {
		Register[*testUser, *testUser, *testUser](consts.Phase("test_register_empty_route"), "  ", &svc{})
	})
}

// TestResolveReturnsBaseOnTypeMismatch covers the wiring-bug path opened by
// the type-free key: a hand-written registration can disagree with the
// resolving factory's type parameters, and the mismatch must degrade to the
// no-op Base instead of panicking mid-request.
func TestResolveReturnsBaseOnTypeMismatch(t *testing.T) {
	type svc struct {
		Base[*testUser, *testUser, *testUser]
	}

	phase := consts.Phase("test_resolve_type_mismatch")
	Register[*testUser, *testUser, *testUser](phase, "samples", &svc{})

	var resolved any
	require.NotPanics(t, func() {
		resolved = Resolve[*testRecord, *testRecord, *testRecord](Key(phase, "samples"))
	})
	_, ok := resolved.(*Base[*testRecord, *testRecord, *testRecord])
	require.True(t, ok, "type mismatch should degrade to the no-op Base")
}
