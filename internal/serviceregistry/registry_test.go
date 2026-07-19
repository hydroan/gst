package serviceregistry

import (
	"testing"

	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/logger/zap"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
)

func TestRegisterAndResolve(t *testing.T) {
	logger.Service = zap.New("")

	type svc struct {
		Base[*testUser, *testUser, *testUser]
	}

	registered := &svc{}
	phase := consts.Phase("test_register_and_resolve")

	Register[*testUser, *testUser, *testUser](phase, registered)
	resolved := Resolve[*testUser, *testUser, *testUser](KeyFor[*testUser, *testUser, *testUser](phase))

	require.Same(t, registered, resolved)
	require.NotNil(t, registered.Logger)
}

func TestResolveReturnsBaseWhenServiceMissing(t *testing.T) {
	key := KeyFor[*testUser, *testUser, *testUser](consts.Phase("test_missing_service"))
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
	key := KeyFor[*testUser, *testUser, *testUser](phase)

	resolved := Resolve[*testUser, *testUser, *testUser](key)
	_, ok := resolved.(*Base[*testUser, *testUser, *testUser])
	require.True(t, ok, "missing service should resolve to the no-op Base")

	registered := &svc{}
	Register[*testUser, *testUser, *testUser](phase, registered)

	require.Same(t, registered, Resolve[*testUser, *testUser, *testUser](key))
}
