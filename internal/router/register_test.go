package router_test

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/router"
	"github.com/stretchr/testify/require"
)

// TestRegisterPanicsOnABlankRouteOrNoVerbs pins the two registrations Register
// refuses as declaration mistakes: a blank route, and a route given no verbs.
// Both panic as they register, so the mistake stops the start instead of
// leaving an endpoint that answers 404.
func TestRegisterPanicsOnABlankRouteOrNoVerbs(t *testing.T) {
	group := gin.New().Group(consts.APIPathPrefix)

	require.PanicsWithValue(t, "router: register requires a non-empty route", func() {
		router.Register[*modelregistry.Empty, *modelregistry.Empty, *modelregistry.Empty](group, "  ", nil, consts.Create, consts.List)
	})
	require.PanicsWithValue(t, `router: register of route "samples" requires at least one verb`, func() {
		router.Register[*modelregistry.Empty, *modelregistry.Empty, *modelregistry.Empty](group, "samples", nil)
	})
}
