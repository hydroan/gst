package module

import (
	"testing"

	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
)

// TestCRUDRoute pins the phase-to-route layout shared by service registration
// and router registration: both derive the registry key from this route, so a
// drift between them would silently detach services from their handlers.
func TestCRUDRoute(t *testing.T) {
	cases := []struct {
		phase consts.Phase
		want  string
	}{
		{consts.PHASE_CREATE, "samples"},
		{consts.PHASE_LIST, "samples"},
		{consts.PHASE_DELETE, "samples/:id"},
		{consts.PHASE_UPDATE, "samples/:id"},
		{consts.PHASE_PATCH, "samples/:id"},
		{consts.PHASE_GET, "samples/:id"},
		{consts.PHASE_CREATE_MANY, "samples/batch"},
		{consts.PHASE_DELETE_MANY, "samples/batch"},
		{consts.PHASE_UPDATE_MANY, "samples/batch"},
		{consts.PHASE_PATCH_MANY, "samples/batch"},
	}
	for _, c := range cases {
		require.Equal(t, c.want, crudRoute("samples", "id", c.phase), "phase %s", c.phase)
	}
}
