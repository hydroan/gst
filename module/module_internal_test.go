package module

import (
	"testing"

	"github.com/hydroan/gst/internal/apiaction"
	"github.com/stretchr/testify/require"
)

// TestCRUDRoute pins the phase-to-route layout shared by service registration
// and router registration: both derive the registry key from this route, so a
// drift between them would silently detach services from their handlers. It
// is the layout gg gen registers a generated project's routes by (see
// apiaction.RouteSuffix), Import, Export and SSE included.
func TestCRUDRoute(t *testing.T) {
	cases := []struct {
		phase apiaction.Phase
		want  string
	}{
		{apiaction.Create, "samples"},
		{apiaction.List, "samples"},
		{apiaction.SSE, "samples"},
		{apiaction.Delete, "samples/:id"},
		{apiaction.Update, "samples/:id"},
		{apiaction.Patch, "samples/:id"},
		{apiaction.Get, "samples/:id"},
		{apiaction.CreateMany, "samples/batch"},
		{apiaction.DeleteMany, "samples/batch"},
		{apiaction.UpdateMany, "samples/batch"},
		{apiaction.PatchMany, "samples/batch"},
		{apiaction.Import, "samples/import"},
		{apiaction.Export, "samples/export"},
	}
	for _, c := range cases {
		require.Equal(t, c.want, crudRoute("samples", "id", c.phase), "phase %s", c.phase)
	}
}

// TestCRUDAndExactRefuseAPhaseWithoutAnHTTPRoute pins that a phase no HTTP
// route serves, a hook phase, is refused where the module declares it, by
// name, instead of registering a service that no route reaches, and that
// every action is taken, the Stream among them.
func TestCRUDAndExactRefuseAPhaseWithoutAnHTTPRoute(t *testing.T) {
	require.PanicsWithValue(t, "module.CRUD: phase delete_after has no HTTP route", func() { CRUD(apiaction.List, apiaction.DeleteAfter) })
	require.PanicsWithValue(t, "module.Exact: phase create_before has no HTTP route", func() { Exact(apiaction.CreateBefore) })
	require.NotPanics(t, func() { CRUD(apiaction.Import, apiaction.Export, apiaction.SSE) })
	require.NotPanics(t, func() { Exact(apiaction.Stream) })
}
