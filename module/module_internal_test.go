package module

import (
	"testing"

	"github.com/hydroan/gst/internal/consts"
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
		{consts.Create, "samples"},
		{consts.List, "samples"},
		{consts.Delete, "samples/:id"},
		{consts.Update, "samples/:id"},
		{consts.Patch, "samples/:id"},
		{consts.Get, "samples/:id"},
		{consts.CreateMany, "samples/batch"},
		{consts.DeleteMany, "samples/batch"},
		{consts.UpdateMany, "samples/batch"},
		{consts.PatchMany, "samples/batch"},
	}
	for _, c := range cases {
		require.Equal(t, c.want, crudRoute("samples", "id", c.phase), "phase %s", c.phase)
	}
}
