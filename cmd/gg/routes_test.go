package main

import (
	"bytes"
	"go/ast"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gggen"
	"github.com/stretchr/testify/require"
)

// TestParseModelRoutesReadsTheRuntimeMethodOfEveryVerb builds a router file
// the way gg gen writes router/router.gen.go, one route per action phase, and
// reads each route back under the method the framework router registers the
// verb by, consts.Phase.HTTPMethod (see the router's own test of that).
func TestParseModelRoutesReadsTheRuntimeMethodOfEveryVerb(t *testing.T) {
	phases := []consts.Phase{
		consts.Create, consts.Delete, consts.Update, consts.Patch,
		consts.List, consts.Get,
		consts.CreateMany, consts.DeleteMany, consts.UpdateMany, consts.PatchMany,
		consts.Import, consts.Export, consts.SSE,
	}
	stmts := make([]ast.Stmt, 0, len(phases))
	want := make(map[string]string, len(phases))
	for _, phase := range phases {
		path := consts.APIPath("samples/" + string(phase))
		stmts = append(stmts, gggen.StmtRouterRegister("model", "Sample", "*Sample", "*Sample", "model", "Auth", path, "", phase.Name()))
		want[path] = phase.HTTPMethod()
	}
	code, err := gggen.BuildRouterFile("router", "model", map[string]string{"tmpapp/model": ""}, stmts...)
	require.NoError(t, err)
	routerFile := filepath.Join(t.TempDir(), ggconst.FileRouterGen)
	require.NoError(t, os.WriteFile(routerFile, []byte(code), 0o600))

	routes, err := parseModelRoutesFromProject(routerFile, t.TempDir())
	require.NoError(t, err)
	got := make(map[string]string, len(routes))
	for _, route := range routes {
		got[route.Path] = route.Method
	}
	require.Equal(t, want, got)
}

// TestRoutesListEveryRouteByItsFullPath verifies both route views print each
// route by the path it is served at, prefix included, with no base line the
// paths would have to be read against.
func TestRoutesListEveryRouteByItsFullPath(t *testing.T) {
	routes := []modelRoute{
		{Model: "*sample.Record", Source: "sample/record.go", Path: "/api/samples", Method: "GET", Phase: "List", Scope: "auth"},
	}
	views := []struct {
		name  string
		print func(w io.Writer, routes []modelRoute, opts modelRoutesPrintOptions)
	}{
		{"router", printRouterRoutes},
		{"model", printModelRoutes},
	}
	for _, view := range views {
		t.Run(view.name, func(t *testing.T) {
			var buf bytes.Buffer
			view.print(&buf, routes, modelRoutesPrintOptions{})
			got := buf.String()
			require.Contains(t, got, " /api/samples\n")
			require.NotContains(t, got, "base:")
		})
	}
}
