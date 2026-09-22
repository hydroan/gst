package main

import (
	"bytes"
	"go/ast"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/codegen/constants"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/stretchr/testify/require"
)

// TestParseModelRoutesReadsTheRuntimeMethodOfEveryVerb builds a router file
// the way gg gen writes router/router.gen.go, one route per action phase, and
// reads each route back under the method the framework router registers the
// verb by, consts.HTTPVerb.HTTPMethod (see the router's own test of that).
func TestParseModelRoutesReadsTheRuntimeMethodOfEveryVerb(t *testing.T) {
	phases := []consts.Phase{
		consts.PHASE_CREATE, consts.PHASE_DELETE, consts.PHASE_UPDATE, consts.PHASE_PATCH,
		consts.PHASE_LIST, consts.PHASE_GET,
		consts.PHASE_CREATE_MANY, consts.PHASE_DELETE_MANY, consts.PHASE_UPDATE_MANY, consts.PHASE_PATCH_MANY,
		consts.PHASE_IMPORT, consts.PHASE_EXPORT, consts.PHASE_SSE,
	}
	stmts := make([]ast.Stmt, 0, len(phases))
	want := make(map[string]string, len(phases))
	for _, phase := range phases {
		path := "samples/" + string(phase)
		stmts = append(stmts, gen.StmtRouterRegister("model", "Sample", "*Sample", "*Sample", "model", "Auth", path, "", phase.MethodName()))
		want[path] = phase.ToHTTPVerb().HTTPMethod()
	}
	code, err := gen.BuildRouterFile("router", "model", map[string]string{"tmpapp/model": ""}, stmts...)
	require.NoError(t, err)
	routerFile := filepath.Join(t.TempDir(), constants.FileRouterGen)
	require.NoError(t, os.WriteFile(routerFile, []byte(code), 0o600))

	routes, err := parseModelRoutesFromProject(routerFile, t.TempDir())
	require.NoError(t, err)
	got := make(map[string]string, len(routes))
	for _, route := range routes {
		got[route.Path] = route.Method
	}
	require.Equal(t, want, got)
}

// TestRoutesHeaderShowsAPIBasePath verifies both route views print the shared
// API mount prefix in their summary header, so the relative paths listed below
// it are unambiguous about where they are actually mounted.
func TestRoutesHeaderShowsAPIBasePath(t *testing.T) {
	routes := []modelRoute{
		{Model: "*sample.Record", Source: "sample/record.go", Path: "samples", Method: "GET", Phase: "List", Scope: "auth"},
	}
	views := []struct {
		name  string
		print func(w io.Writer, routes []modelRoute, opts modelRoutesPrintOptions)
	}{
		{"router", printRouterRoutes},
		{"model", printModelRoutes},
	}
	want := "base: " + consts.APIPathPrefix
	for _, view := range views {
		t.Run(view.name, func(t *testing.T) {
			var buf bytes.Buffer
			view.print(&buf, routes, modelRoutesPrintOptions{})
			if got := buf.String(); !strings.Contains(got, want) {
				t.Errorf("%s view header missing %q\n%s", view.name, want, got)
			}
		})
	}
}
