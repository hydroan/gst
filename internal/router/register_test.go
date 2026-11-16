package router_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/router"
	"github.com/stretchr/testify/require"
)

// sampleBinder is an interface with methods, a request type no request body
// decodes into.
type sampleBinder interface{ Bind() }

// TestRegisterPanicsOnDeclarationMistakes pins the registrations Register
// refuses as declaration mistakes: a blank route, a route given no phases, a
// phase no HTTP route serves, a hook phase, and a request type no request
// body decodes into. Each panics as it registers, so the mistake stops the
// start instead of leaving an endpoint that answers 404, refuses every
// request or was never registered.
func TestRegisterPanicsOnDeclarationMistakes(t *testing.T) {
	group := gin.New().Group("")

	require.PanicsWithValue(t, "router: register requires a non-empty route", func() {
		router.Register[*modelregistry.Empty, *modelregistry.Empty, *modelregistry.Empty](group, "  ", nil, apiaction.Create, apiaction.List)
	})
	require.PanicsWithValue(t, `router: register of route "samples" requires at least one phase`, func() {
		router.Register[*modelregistry.Empty, *modelregistry.Empty, *modelregistry.Empty](group, "samples", nil)
	})
	require.PanicsWithValue(t, `router: register of route "/api/samples": no HTTP route serves the phase CreateBefore, DeleteAfter; a hook phase runs inside its action`, func() {
		router.Register[*modelregistry.Empty, *modelregistry.Empty, *modelregistry.Empty](group, "samples", nil, apiaction.Create, apiaction.DeleteAfter, apiaction.CreateBefore)
	})
	require.PanicsWithValue(t, `controller: route "/api/samples/:id/bind": request type *router_test.sampleBinder is an interface with methods or a pointer to one, which no request body decodes into; declare a concrete type, or any`, func() {
		router.Register[*modelregistry.Empty, *sampleBinder, *modelregistry.Empty](group, "samples/:id/bind", nil, apiaction.Create)
	})
}

// TestRegisterServesEveryPhaseUnderItsMethod pins the method the router serves
// each phase under to apiaction.Phase.HTTPMethod, the table gg routes, gg
// route-tree and gg gen's route ignore rules read as well: the engine routes
// the method, and Routes lists the route under it. A Stream action is the
// one exception to the list: served under the GET that opens its connection,
// it is listed under the stream word, what it is authorized by.
func TestRegisterServesEveryPhaseUnderItsMethod(t *testing.T) {
	engine := gin.New()
	group := engine.Group("")
	phases := []apiaction.Phase{
		apiaction.Create, apiaction.Delete, apiaction.Update, apiaction.Patch, apiaction.List, apiaction.Get,
		apiaction.CreateMany, apiaction.DeleteMany, apiaction.UpdateMany, apiaction.PatchMany,
		apiaction.Import, apiaction.Export, apiaction.SSE, apiaction.Stream,
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
		path := apiaction.APIPath("phase-methods/" + string(phase))
		require.Equal(t, phase.HTTPMethod(), served[path], "phase %s", phase)
		listed := phase.HTTPMethod()
		if phase == apiaction.Stream {
			listed = apiaction.MethodStream
		}
		require.Equal(t, []string{listed}, recorded[path], "phase %s", phase)
	}
}

// TestRouteAnswersTheMethodARouteIsListedUnder pins what Route tells a
// middleware of the request in hand: the HTTP method of the request and the
// route pattern it matched, and the stream word in place of the GET for a
// request to the route of a Stream action, whether or not the request asks
// for an upgrade. An SSE stream is a GET like any other, another action of
// a stream's own route answers by its method, and a request no route
// matched has no route.
func TestRouteAnswersTheMethodARouteIsListedUnder(t *testing.T) {
	engine := gin.New()
	var method, route string
	probe := func(c *gin.Context) {
		method, route = router.Route(c)
		c.Abort()
	}
	group := engine.Group("", probe)
	engine.NoRoute(probe)
	router.Register[*modelregistry.Empty, *modelregistry.Empty, *modelregistry.Empty](group, "route-methods/:id/tail", nil, apiaction.Stream, apiaction.Create)
	router.Register[*modelregistry.Empty, *modelregistry.Empty, *modelregistry.Empty](group, "route-methods/:id/events", nil, apiaction.SSE)
	router.Register[*modelregistry.Empty, *modelregistry.Empty, *modelregistry.Empty](group, "route-methods/:id", nil, apiaction.Get, apiaction.Update)

	for _, tt := range []struct {
		name       string
		method     string
		path       string
		upgrade    bool
		wantMethod string
		wantRoute  string
	}{
		{name: "the handshake of a stream", method: http.MethodGet, path: "/api/route-methods/7/tail", upgrade: true, wantMethod: apiaction.MethodStream, wantRoute: "/api/route-methods/:id/tail"},
		{name: "the route of a stream asked without an upgrade", method: http.MethodGet, path: "/api/route-methods/7/tail", wantMethod: apiaction.MethodStream, wantRoute: "/api/route-methods/:id/tail"},
		{name: "another action of the route of a stream", method: http.MethodPost, path: "/api/route-methods/7/tail", wantMethod: http.MethodPost, wantRoute: "/api/route-methods/:id/tail"},
		{name: "an SSE stream", method: http.MethodGet, path: "/api/route-methods/7/events", wantMethod: http.MethodGet, wantRoute: "/api/route-methods/:id/events"},
		{name: "an action served under GET", method: http.MethodGet, path: "/api/route-methods/7", wantMethod: http.MethodGet, wantRoute: "/api/route-methods/:id"},
		{name: "an action served under PUT", method: http.MethodPut, path: "/api/route-methods/7", wantMethod: http.MethodPut, wantRoute: "/api/route-methods/:id"},
		{name: "a request no route matched", method: http.MethodGet, path: "/api/route-methods", wantMethod: http.MethodGet, wantRoute: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, nil)
			if tt.upgrade {
				req.Header.Set("Connection", "Upgrade")
				req.Header.Set("Upgrade", "websocket")
			}
			method, route = "", "unset"
			engine.ServeHTTP(httptest.NewRecorder(), req)
			require.Equal(t, tt.wantMethod, method)
			require.Equal(t, tt.wantRoute, route)
		})
	}
}
