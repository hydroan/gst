package router_test

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/router"
	"github.com/stretchr/testify/require"
)

// sampleBinder is an interface with methods, a request type no request body
// decodes into.
type sampleBinder interface{ Bind() }

// TestRegisterPanicsOnDeclarationMistakes pins the registrations Register
// refuses as declaration mistakes: a blank route, a route given no phases, a
// phase no HTTP route serves, a hook phase or a Stream, and a request type
// no request body decodes into. Each panics as it registers, so the mistake
// stops the start instead of leaving an endpoint that answers 404, refuses
// every request or was never registered.
func TestRegisterPanicsOnDeclarationMistakes(t *testing.T) {
	group := gin.New().Group("")

	require.PanicsWithValue(t, "router: register requires a non-empty route", func() {
		router.Register[*modelregistry.Empty, *modelregistry.Empty, *modelregistry.Empty](group, "  ", nil, consts.Create, consts.List)
	})
	require.PanicsWithValue(t, `router: register of route "samples" requires at least one phase`, func() {
		router.Register[*modelregistry.Empty, *modelregistry.Empty, *modelregistry.Empty](group, "samples", nil)
	})
	require.PanicsWithValue(t, `router: register of route "/api/samples": no HTTP route serves the phase CreateBefore, Stream; a hook phase runs inside its action and a Stream is served over gRPC alone`, func() {
		router.Register[*modelregistry.Empty, *modelregistry.Empty, *modelregistry.Empty](group, "samples", nil, consts.Create, consts.CreateBefore, consts.Stream)
	})
	require.PanicsWithValue(t, `controller: route "/api/samples/:id/bind": request type *router_test.sampleBinder is an interface with methods or a pointer to one, which no request body decodes into; declare a concrete type, or any`, func() {
		router.Register[*modelregistry.Empty, *sampleBinder, *modelregistry.Empty](group, "samples/:id/bind", nil, consts.Create)
	})
}

// TestRegisterServesEveryPhaseUnderItsMethod pins the method the router serves
// each phase under to consts.Phase.HTTPMethod, the table gg routes, gg
// route-tree and gg gen's route ignore rules read as well: the engine routes
// the method, and Routes records it.
func TestRegisterServesEveryPhaseUnderItsMethod(t *testing.T) {
	engine := gin.New()
	group := engine.Group("")
	phases := []consts.Phase{
		consts.Create, consts.Delete, consts.Update, consts.Patch, consts.List, consts.Get,
		consts.CreateMany, consts.DeleteMany, consts.UpdateMany, consts.PatchMany,
		consts.Import, consts.Export, consts.SSE,
	}
	for _, phase := range phases {
		router.Register[*modelregistry.Empty, *modelregistry.Empty, *modelregistry.Empty](group, "phase-methods/"+string(phase), nil, phase)
	}

	served := make(map[string]string, len(phases))
	for _, route := range engine.Routes() {
		served[route.Path] = route.Method
	}
	recorded := router.Routes()
	for _, phase := range phases {
		path := consts.APIPath("phase-methods/" + string(phase))
		require.Equal(t, phase.HTTPMethod(), served[path], "phase %s", phase)
		require.Equal(t, []string{phase.HTTPMethod()}, recorded[path], "phase %s", phase)
	}
}
