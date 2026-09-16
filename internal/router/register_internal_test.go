package router

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/stretchr/testify/require"
)

// TestRegisterWithoutARouteOrVerbsRegistersNothing pins the two registrations
// Register turns away as it documents: a blank route, and a route given no
// verbs. Neither reaches the router group nor the route registry Routes reads.
func TestRegisterWithoutARouteOrVerbsRegistersNothing(t *testing.T) {
	cases := []struct {
		name  string
		route string
		verbs []consts.HTTPVerb
	}{
		{name: "a blank route", route: "  ", verbs: []consts.HTTPVerb{consts.Create, consts.List}},
		{name: "no verbs", route: "samples"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			engine := gin.New()
			registered := Routes()

			Register[*modelregistry.Empty, *modelregistry.Empty, *modelregistry.Empty](engine.Group(consts.APIPathPrefix), tc.route, nil, tc.verbs...)

			require.Empty(t, engine.Routes())
			require.Equal(t, registered, Routes())
		})
	}
}
