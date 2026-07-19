package service

import (
	"reflect"
	"testing"

	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/logger/zap"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
)

type testUser struct {
	Name string
	model.Base
}

func TestRegister(t *testing.T) {
	type svc = Base[*testUser, *testUser, *testUser]

	t.Run("pointer", func(t *testing.T) {
		Register[*svc](consts.PHASE_CREATE)
	})
	t.Run("struct", func(t *testing.T) {
		Register[svc](consts.PHASE_CREATE)
	})
}

func TestBaseAliasesServiceRegistryBase(t *testing.T) {
	require.Equal(
		t,
		reflect.TypeFor[serviceregistry.Base[*testUser, *testUser, *testUser]](),
		reflect.TypeFor[Base[*testUser, *testUser, *testUser]](),
	)
}

func TestRegister2(t *testing.T) {
	type svc = struct {
		Base[*testUser, *testUser, *testUser]
	}

	t.Run("pointer", func(t *testing.T) {
		Register[*svc](consts.PHASE_CREATE)
	})
	t.Run("struct", func(t *testing.T) {
		Register[svc](consts.PHASE_CREATE)
	})
}

func TestRegister3(t *testing.T) {
	type svc = struct {
		*Base[*testUser, *testUser, *testUser]
	}

	t.Run("pointer", func(t *testing.T) {
		Register[*svc](consts.PHASE_CREATE)
	})
	t.Run("struct", func(t *testing.T) {
		Register[svc](consts.PHASE_CREATE)
	})
}

func TestService(t *testing.T) {
	logger.Service = zap.New("")

	type svc = Base[*testUser, *testUser, *testUser]
	Register[*svc](consts.PHASE_CREATE)
	Register[*svc](consts.PHASE_DELETE)

	t.Run("logger", func(t *testing.T) {
		for _, phase := range []consts.Phase{consts.PHASE_CREATE, consts.PHASE_DELETE} {
			key := serviceregistry.KeyFor[*testUser, *testUser, *testUser](phase)
			s, ok := serviceregistry.Resolve[*testUser, *testUser, *testUser](key).(*svc)
			require.True(t, ok)
			require.NotNil(t, s)
			require.NotNil(t, s.Logger) // service logger was set
		}
	})
}
