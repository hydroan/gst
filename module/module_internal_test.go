package module

import (
	"testing"

	"github.com/hydroan/gst/internal/consts"
	"github.com/stretchr/testify/require"
)

// TestCRUDRoute pins the phase-to-route layout shared by service registration
// and router registration: both derive the registry key from this route, so a
// drift between them would silently detach services from their handlers. It
// is the layout gg gen registers a generated project's routes by (see
// consts.RouteSuffix), Import, Export and SSE included.
func TestCRUDRoute(t *testing.T) {
	cases := []struct {
		phase consts.Phase
		want  string
	}{
		{consts.Create, "samples"},
		{consts.List, "samples"},
		{consts.SSE, "samples"},
		{consts.Delete, "samples/:id"},
		{consts.Update, "samples/:id"},
		{consts.Patch, "samples/:id"},
		{consts.Get, "samples/:id"},
		{consts.CreateMany, "samples/batch"},
		{consts.DeleteMany, "samples/batch"},
		{consts.UpdateMany, "samples/batch"},
		{consts.PatchMany, "samples/batch"},
		{consts.Import, "samples/import"},
		{consts.Export, "samples/export"},
	}
	for _, c := range cases {
		require.Equal(t, c.want, crudRoute("samples", "id", c.phase), "phase %s", c.phase)
	}
}

// TestCRUDAndExactRefuseAPhaseWithoutAnHTTPRoute pins that a phase no HTTP
// route serves, Stream, served over gRPC alone, and the hook phases, is
// refused where the module declares it, by name, instead of registering a
// service that no route reaches.
func TestCRUDAndExactRefuseAPhaseWithoutAnHTTPRoute(t *testing.T) {
	require.PanicsWithValue(t, "module.CRUD: phase stream has no HTTP route", func() { CRUD(consts.List, consts.Stream) })
	require.PanicsWithValue(t, "module.Exact: phase create_before has no HTTP route", func() { Exact(consts.CreateBefore) })
	require.NotPanics(t, func() { CRUD(consts.Import, consts.Export, consts.SSE) })
}
