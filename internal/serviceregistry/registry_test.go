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
	resolved := Resolve[*testUser, *testUser, *testUser](phase)

	require.Same(t, registered, resolved)
	require.NotNil(t, registered.Logger)
}

func TestResolveReturnsBaseWhenServiceMissing(t *testing.T) {
	resolved := Resolve[*testUser, *testUser, *testUser](consts.Phase("test_missing_service"))

	_, ok := resolved.(*Base[*testUser, *testUser, *testUser])
	require.True(t, ok)
}
