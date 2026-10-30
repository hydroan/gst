package main

import (
	"bytes"
	"testing"

	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/gggen"
	"github.com/stretchr/testify/require"
)

// TestParseRouteTreeFromFileReadsGeneratedRoutes parses a router file built
// the way gg gen writes router/router.gen.go and finds every route it
// registers, with the HTTP method of each action: export and SSE routes are
// served over GET like the rest of the reads.
func TestParseRouteTreeFromFileReadsGeneratedRoutes(t *testing.T) {
	chdirToRoutedProject(t,
		gggen.StmtRouterRegister("model", "Record", "*Record", "*Record", "model", "Auth", "/api/records", "", "Create"),
		gggen.StmtRouterRegister("model", "Record", "*Record", "*Record", "model", "Auth", "/api/records", "", "List"),
		gggen.StmtRouterRegister("model", "Record", "*Record", "*Record", "model", "Auth", "/api/records/:rec", "rec", "Get"),
		gggen.StmtRouterRegister("model", "Record", "*Record", "*Record", "model", "Pub", "/api/records/:rec", "rec", "Delete"),
		gggen.StmtRouterRegister("model", "Record", "*Record", "*Record", "model", "Auth", "/api/records/export", "", "Export"),
		gggen.StmtRouterRegister("model", "Notice", "*Notice", "*Notice", "model", "Auth", "/api/notices", "", "SSE"),
	)

	routes, err := parseRouteTreeFromFile()
	require.NoError(t, err)
	require.Equal(t, map[string][]string{
		"/api/records":        {"POST", "GET"},
		"/api/records/:rec":   {"GET", "DELETE"},
		"/api/records/export": {"GET"},
		"/api/notices":        {"GET"},
	}, routes)
}

// TestRouteTreeRunPrintsTheRoutesAsAURLTree pins gg route-tree over the
// router file of a project: the routes as a tree of their path segments,
// each route with its methods sorted, and a route with routes under it
// marked on a line of its own; a filter keeps the routes whose path holds
// it, and one keeping nothing says which it was.
func TestRouteTreeRunPrintsTheRoutesAsAURLTree(t *testing.T) {
	chdirToRoutedProject(t,
		gggen.StmtRouterRegister("model", "Record", "*Record", "*Record", "model", "Auth", apiaction.APIPath("records"), "", apiaction.Create.Name()),
		gggen.StmtRouterRegister("model", "Record", "*Record", "*Record", "model", "Auth", apiaction.APIPath("records"), "", apiaction.List.Name()),
		gggen.StmtRouterRegister("model", "Record", "*Record", "*Record", "model", "Auth", apiaction.APIPath("records/:id"), "id", apiaction.Get.Name()),
		gggen.StmtRouterRegister("model", "Item", "*Item", "*Item", "model", "Pub", apiaction.APIPath("items"), "", apiaction.List.Name()),
	)

	var out bytes.Buffer
	require.NoError(t, routeTreeRun(&out, ""))
	require.Equal(t, "Route Tree Structure:\n=====================\n└─ api/\n   ├─ items [GET]\n   └─ records/\n      ● [GET, POST]\n      └─ :id [GET]\n", out.String())

	out.Reset()
	require.NoError(t, routeTreeRun(&out, "/items"))
	require.Equal(t, "Route Tree Structure:\n=====================\n└─ api/\n   └─ items [GET]\n", out.String())

	out.Reset()
	require.NoError(t, routeTreeRun(&out, "nosuch"))
	require.Equal(t, "No routes found matching filter: nosuch\n", out.String())
}
