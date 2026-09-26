package consts_test

import (
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/stretchr/testify/require"
)

// TestAPIPathServesEveryRouteUnderThePrefix pins the examples of the APIPath
// doc comment: a route with or without a leading slash, or already carrying
// the prefix, is served under the prefix once, a trailing slash dropped.
func TestAPIPathServesEveryRouteUnderThePrefix(t *testing.T) {
	for _, route := range []string{"records/:id", "/records/:id", "/api/records/:id", "records/:id/"} {
		require.Equal(t, "/api/records/:id", consts.APIPath(route), route)
	}
	require.Equal(t, "/api/owners/:owner/records", consts.APIPath("/owners/:owner/records"))
}
