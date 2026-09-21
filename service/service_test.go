package service_test

import (
	"fmt"
	"reflect"
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
		service.Register[*base](consts.PHASE_CREATE, "samples/pointer")
		requireResolvesTo[base](t, consts.PHASE_CREATE, "samples/pointer")
	})
	t.Run("base by value", func(t *testing.T) {
		service.Register[base](consts.PHASE_CREATE, "samples/struct")
		requireResolvesTo[base](t, consts.PHASE_CREATE, "samples/struct")
	})
	t.Run("canonical struct by pointer", func(t *testing.T) {
		service.Register[*canonical](consts.PHASE_CREATE, "records/pointer")
		requireResolvesTo[canonical](t, consts.PHASE_CREATE, "records/pointer")
	})
	t.Run("canonical struct by value", func(t *testing.T) {
		service.Register[canonical](consts.PHASE_CREATE, "records/struct")
		requireResolvesTo[canonical](t, consts.PHASE_CREATE, "records/struct")
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
	service.Register[*base](consts.PHASE_CREATE, "samples/service")
	service.Register[*base](consts.PHASE_DELETE, "samples/service")
	service.Register[*canonical](consts.PHASE_CREATE, "records/service")

	for _, phase := range []consts.Phase{consts.PHASE_CREATE, consts.PHASE_DELETE} {
		s, ok := serviceregistry.Resolve[*testUser, *testUser, *testUser](serviceregistry.Key(phase, "samples/service")).(*base)
		require.True(t, ok)
		require.NotNil(t, s.Logger, "phase %s", phase)
	}
	s, ok := serviceregistry.Resolve[*testUser, *testUser, *testUser](serviceregistry.Key(consts.PHASE_CREATE, "records/service")).(*canonical)
	require.True(t, ok)
	require.NotNil(t, s.Logger)
}

// requireResolvesTo asserts that the route and phase resolve to a *S instance.
func requireResolvesTo[S any](t *testing.T, phase consts.Phase, route string) {
	t.Helper()

	got := serviceregistry.Resolve[*testUser, *testUser, *testUser](serviceregistry.Key(phase, route))
	require.Equal(t, reflect.TypeFor[*S](), reflect.TypeOf(got), "route %q phase %q", route, phase)
}
