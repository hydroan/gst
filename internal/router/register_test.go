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

// TestRegisterServesEveryVerbUnderItsMethod pins the method the router serves
// each verb under to consts.HTTPVerb.HTTPMethod, the table gg routes, gg
// route-tree and gg gen's route ignore rules read as well: the engine routes
// the method, and Routes records it.
func TestRegisterServesEveryVerbUnderItsMethod(t *testing.T) {
	engine := gin.New()
	group := engine.Group(consts.APIPathPrefix)
	verbs := []consts.HTTPVerb{
		consts.Create, consts.Delete, consts.Update, consts.Patch, consts.List, consts.Get,
		consts.CreateMany, consts.DeleteMany, consts.UpdateMany, consts.PatchMany,
		consts.Import, consts.Export, consts.SSE,
	}
	for _, verb := range verbs {
		router.Register[*modelregistry.Empty, *modelregistry.Empty, *modelregistry.Empty](group, "verb-methods/"+string(verb), nil, verb)
	}

	served := make(map[string]string, len(verbs))
	for _, route := range engine.Routes() {
		served[route.Path] = route.Method
	}
	recorded := router.Routes()
	for _, verb := range verbs {
		path := consts.APIPathPrefix + "/verb-methods/" + string(verb)
		require.Equal(t, verb.HTTPMethod(), served[path], "verb %s", verb)
		require.Equal(t, []string{verb.HTTPMethod()}, recorded[path], "verb %s", verb)
	}
}
