package apiaction_test

import (
	"testing"

	"github.com/hydroan/gst/internal/apiaction"
	"github.com/stretchr/testify/require"
)

// TestAPIPathServesEveryRouteUnderThePrefix pins the examples of the APIPath
// doc comment: a route with or without a leading slash, or already carrying
// the prefix with or without one, is served under the prefix once, a
// trailing slash dropped; a blank route and the prefix alone name the root
// of the prefix.
func TestAPIPathServesEveryRouteUnderThePrefix(t *testing.T) {
	for _, route := range []string{"records/:id", "/records/:id", "api/records/:id", "/api/records/:id", "records/:id/"} {
		require.Equal(t, "/api/records/:id", apiaction.APIPath(route), route)
	}
	require.Equal(t, "/api/owners/:owner/records", apiaction.APIPath("/owners/:owner/records"))
	for _, route := range []string{"", "/", "api", "/api", "/api/"} {
		require.Equal(t, "/api/", apiaction.APIPath(route), route)
	}
}

// TestLastRouteParamNamesTheLastParameterSegment pins the examples of the
// LastRouteParam doc comment: the last :name of a route whatever follows it,
// spaces around a segment passed over, and "" for a route naming none or
// writing a colon alone.
func TestLastRouteParamNamesTheLastParameterSegment(t *testing.T) {
	require.Equal(t, "record", apiaction.LastRouteParam("/api/records/:record"))
	require.Equal(t, "id", apiaction.LastRouteParam("iam/admin/users/:id/sessions"))
	require.Equal(t, "item", apiaction.LastRouteParam("/api/items/ :item /notes"))
	require.Empty(t, apiaction.LastRouteParam("records"))
	require.Empty(t, apiaction.LastRouteParam("records/:"))
}

// TestRouteSuffix pins what the CRUD layout appends to a route for each
// action, the one table module.Use and gg gen register routes by: the item
// parameter for the actions on one row, "batch" for the batch actions,
// "import" and "export" for Import and Export, and nothing for the actions
// served on the route itself, SSE included.
func TestRouteSuffix(t *testing.T) {
	cases := []struct {
		phase apiaction.Phase
		want  string
	}{
		{apiaction.Create, ""},
		{apiaction.List, ""},
		{apiaction.SSE, ""},
		{apiaction.Delete, ":id"},
		{apiaction.Update, ":id"},
		{apiaction.Patch, ":id"},
		{apiaction.Get, ":id"},
		{apiaction.CreateMany, "batch"},
		{apiaction.DeleteMany, "batch"},
		{apiaction.UpdateMany, "batch"},
		{apiaction.PatchMany, "batch"},
		{apiaction.Import, "import"},
		{apiaction.Export, "export"},
	}
	for _, c := range cases {
		if got := apiaction.RouteSuffix(c.phase, ":id"); got != c.want {
			t.Errorf("RouteSuffix(%s) = %q, want %q", c.phase, got, c.want)
		}
	}
}
