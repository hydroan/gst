package consts_test

import (
	"testing"

	"github.com/hydroan/gst/internal/consts"
	"github.com/stretchr/testify/require"
)

// TestAPIPathServesEveryRouteUnderThePrefix pins the examples of the APIPath
// doc comment: a route with or without a leading slash, or already carrying
// the prefix with or without one, is served under the prefix once, a
// trailing slash dropped; a blank route and the prefix alone name the root
// of the prefix.
func TestAPIPathServesEveryRouteUnderThePrefix(t *testing.T) {
	for _, route := range []string{"records/:id", "/records/:id", "api/records/:id", "/api/records/:id", "records/:id/"} {
		require.Equal(t, "/api/records/:id", consts.APIPath(route), route)
	}
	require.Equal(t, "/api/owners/:owner/records", consts.APIPath("/owners/:owner/records"))
	for _, route := range []string{"", "/", "api", "/api", "/api/"} {
		require.Equal(t, "/api/", consts.APIPath(route), route)
	}
}

// TestLastRouteParamNamesTheLastParameterSegment pins the examples of the
// LastRouteParam doc comment: the last :name of a route whatever follows it,
// spaces around a segment passed over, and "" for a route naming none or
// writing a colon alone.
func TestLastRouteParamNamesTheLastParameterSegment(t *testing.T) {
	require.Equal(t, "record", consts.LastRouteParam("/api/records/:record"))
	require.Equal(t, "id", consts.LastRouteParam("iam/admin/users/:id/sessions"))
	require.Equal(t, "item", consts.LastRouteParam("/api/items/ :item /notes"))
	require.Empty(t, consts.LastRouteParam("records"))
	require.Empty(t, consts.LastRouteParam("records/:"))
}
