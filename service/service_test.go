package service_test

import (
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/logger/zap"
	"github.com/hydroan/gst/service"
	"github.com/stretchr/testify/require"
)

type testUser struct {
	Name string
	modelregistry.Base
}

// TestRegisterResolvesAPointerInstance pins the service types Register
// accepts: service.Base itself and a struct in the form gg gen generates, a
// body that is exactly the service.Base embedding, each named by value or by
// pointer. Either way the registry resolves the route to a pointer instance.
func TestRegisterResolvesAPointerInstance(t *testing.T) {
	type base = service.Base[*testUser, *testUser, *testUser]
	type canonical = struct {
		service.Base[*testUser, *testUser, *testUser]
	}

	t.Run("base by pointer", func(t *testing.T) {
		route := newRoute("samples/pointer")
		service.Register[*base](consts.PHASE_CREATE, route)
		requireResolvesTo[base](t, consts.PHASE_CREATE, route)
	})
	t.Run("base by value", func(t *testing.T) {
		route := newRoute("samples/struct")
		service.Register[base](consts.PHASE_CREATE, route)
		requireResolvesTo[base](t, consts.PHASE_CREATE, route)
	})
	t.Run("canonical struct by pointer", func(t *testing.T) {
		route := newRoute("records/pointer")
		service.Register[*canonical](consts.PHASE_CREATE, route)
		requireResolvesTo[canonical](t, consts.PHASE_CREATE, route)
	})
	t.Run("canonical struct by value", func(t *testing.T) {
		route := newRoute("records/struct")
		service.Register[canonical](consts.PHASE_CREATE, route)
		requireResolvesTo[canonical](t, consts.PHASE_CREATE, route)
	})
}

// TestRegisterPanicsOnAnInterfaceType pins the declaration Register refuses:
// an interface type argument names no service to dispatch to, so registering
// it fails at startup instead of silently registering nothing.
func TestRegisterPanicsOnAnInterfaceType(t *testing.T) {
	type contract = types.Service[*testUser, *testUser, *testUser]

	want := fmt.Sprintf("service: register of route %q phase %q requires a concrete service type, not the interface %s",
		"samples/interface", consts.PHASE_CREATE, reflect.TypeFor[contract]())
	require.PanicsWithValue(t, want, func() {
		service.Register[contract](consts.PHASE_CREATE, "samples/interface")
	})
}

func TestBaseAliasesServiceRegistryBase(t *testing.T) {
	require.Equal(
		t,
		reflect.TypeFor[serviceregistry.Base[*testUser, *testUser, *testUser]](),
		reflect.TypeFor[service.Base[*testUser, *testUser, *testUser]](),
	)
}

// TestRegisterInjectsTheServiceLogger pins the logger a registration gets once
// the service logger exists, through both fields it is injected into: the
// Logger of service.Base itself, and the Logger of the service.Base a struct
// in the generated form embeds.
func TestRegisterInjectsTheServiceLogger(t *testing.T) {
	previous := logger.Service
	logger.Service = zap.New("")
	t.Cleanup(func() { logger.Service = previous })

	type base = service.Base[*testUser, *testUser, *testUser]
	type canonical = struct {
		service.Base[*testUser, *testUser, *testUser]
	}
	samples, records := newRoute("samples/service"), newRoute("records/service")
	service.Register[*base](consts.PHASE_CREATE, samples)
	service.Register[*base](consts.PHASE_DELETE, samples)
	service.Register[*canonical](consts.PHASE_CREATE, records)

	for _, phase := range []consts.Phase{consts.PHASE_CREATE, consts.PHASE_DELETE} {
		s, ok := serviceregistry.Resolve[*testUser, *testUser, *testUser](serviceregistry.Key(phase, samples)).(*base)
		require.True(t, ok)
		require.NotNil(t, s.Logger, "phase %s", phase)
	}
	s, ok := serviceregistry.Resolve[*testUser, *testUser, *testUser](serviceregistry.Key(consts.PHASE_CREATE, records)).(*canonical)
	require.True(t, ok)
	require.NotNil(t, s.Logger)
}

// requireResolvesTo asserts that the route and phase resolve to a *S instance.
func requireResolvesTo[S any](t *testing.T, phase consts.Phase, route string) {
	t.Helper()

	got := serviceregistry.Resolve[*testUser, *testUser, *testUser](serviceregistry.Key(phase, route))
	require.Equal(t, reflect.TypeFor[*S](), reflect.TypeOf(got), "route %q phase %q", route, phase)
}

// routeSeq numbers the routes the tests register. The registry refuses a
// second registration of one route and phase and lives as long as the
// process, so a repeated run (go test -count) registers under routes of its
// own.
var routeSeq atomic.Int64

// newRoute returns a route named after route that no earlier registration of
// the process used.
func newRoute(route string) string {
	return fmt.Sprintf("%s-%d", route, routeSeq.Add(1))
}
