package main

import (
	"bytes"
	"go/ast"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gggen"
	"github.com/stretchr/testify/require"
)

// TestParseModelRoutesReadsTheRuntimeMethodOfEveryPhase builds a router file
// the way gg gen writes router/router.gen.go, one route per action phase, and
// reads each route back under the method the framework router registers the
// phase by, consts.Phase.HTTPMethod (see the router's own test of that).
func TestParseModelRoutesReadsTheRuntimeMethodOfEveryPhase(t *testing.T) {
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
	code, err := gggen.BuildRouterFile("tmpapp", "router", "model", map[string]string{"tmpapp/model": ""}, stmts...)
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
		})
	}
}

// TestRunModelRoutesShowsTheRoutesOfTheScopeAndTextAsked pins the filters of
// gg routes over the router file of a project: --scope keeps the routes of
// one group, the public one under either of its names, a text keeps the
// routes that hold it, a filter that keeps nothing says which it was, and a
// scope gg does not know is refused.
func TestRunModelRoutesShowsTheRoutesOfTheScopeAndTextAsked(t *testing.T) {
	chdirToRoutedProject(t,
		gggen.StmtRouterRegister("model", "Record", "*Record", "*Record", "model", "Auth", consts.APIPath("records"), "", consts.List.Name()),
		gggen.StmtRouterRegister("model", "Item", "*Item", "*Item", "model", "Pub", consts.APIPath("items"), "", consts.List.Name()),
	)
	publicItems := "▶ Router Routes\n  → models: 1, routes: 1, public: 1, auth: 0\n  → scope: public\n\n└─ *model.Item [public]\n   └─ GET    /api/items\n"
	for _, tt := range []struct {
		name, filter, scope, want string
	}{
		{name: "pub", scope: "pub", want: publicItems},
		{name: "public", scope: "Public", want: publicItems},
		{name: "auth", scope: "auth", want: "▶ Router Routes\n  → models: 1, routes: 1, public: 0, auth: 1\n  → scope: auth\n\n└─ *model.Record [auth]\n   └─ GET    /api/records\n"},
		{name: "text", filter: "item", want: "▶ Router Routes\n  → models: 1, routes: 1, public: 1, auth: 0\n  → filter: item\n\n└─ *model.Item [public]\n   └─ GET    /api/items\n"},
		{name: "text_keeping_nothing", filter: "record", scope: "pub", want: "No routes found matching filter: record\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			require.NoError(t, runModelRoutes(&out, tt.filter, &modelRoutesCommandOptions{scope: tt.scope}))
			require.Equal(t, tt.want, out.String())
		})
	}

	var out bytes.Buffer
	require.EqualError(t, runModelRoutes(&out, "", &modelRoutesCommandOptions{scope: "admin"}), "scope must be auth, pub, or public")
}

// TestRunModelRoutesSaysWhenNoRouteIsLeft pins the answers of gg routes that
// list no route: a router file without routes points at gg gen, and a scope
// none of the routes is in says which scope it was.
func TestRunModelRoutesSaysWhenNoRouteIsLeft(t *testing.T) {
	t.Run("no_routes", func(t *testing.T) {
		chdirToRoutedProject(t)
		var out bytes.Buffer
		require.NoError(t, runModelRoutes(&out, "", &modelRoutesCommandOptions{}))
		require.Equal(t, "No routes found. Please run 'gg gen' first to generate routes.\n", out.String())
	})
	t.Run("scope_keeping_nothing", func(t *testing.T) {
		chdirToRoutedProject(t, gggen.StmtRouterRegister("model", "Record", "*Record", "*Record", "model", "Auth", consts.APIPath("records"), "", consts.List.Name()))
		var out bytes.Buffer
		require.NoError(t, runModelRoutes(&out, "", &modelRoutesCommandOptions{scope: "pub"}))
		require.Equal(t, "No routes found matching scope: pub\n", out.String())
	})
}

// TestPrintModelRoutesGroupsTheModelsOfASourceFile pins the model view of a
// source file that declares more than one model: the file line counts them,
// and each model follows with its scope and its own routes.
func TestPrintModelRoutesGroupsTheModelsOfASourceFile(t *testing.T) {
	routes := []modelRoute{
		{Model: "*sample.Record", Source: "sample/record.go", Path: "/api/records", Method: "GET", Phase: "List", Scope: "auth"},
		{Model: "*sample.Entry", Source: "sample/record.go", Path: "/api/entries", Method: "POST", Phase: "Create", Scope: "public"},
	}
	var out bytes.Buffer
	printModelRoutes(&out, routes, modelRoutesPrintOptions{})
	require.Equal(t, "▶ Model Routes\n  → models: 2, routes: 2, public: 1, auth: 1\n\n└─ sample/\n   └─ record.go  (2 models)\n      ├─ *sample.Record [auth]\n      │  └─ GET    /api/records\n      └─ *sample.Entry [public]\n         └─ POST   /api/entries\n", out.String())
}

// chdirToRoutedProject moves the test into a project whose router file
// registers stmts, written the way gg gen writes router/router.gen.go,
// beside an empty model directory.
func chdirToRoutedProject(t *testing.T, stmts ...ast.Stmt) {
	t.Helper()
	code, err := gggen.BuildRouterFile("tmpapp", "router", "model", map[string]string{"tmpapp/model": ""}, stmts...)
	require.NoError(t, err)

	t.Chdir(t.TempDir())
	require.NoError(t, os.Mkdir(ggconst.DirModel, 0o755))
	require.NoError(t, os.Mkdir(ggconst.DirRouter, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ggconst.DirRouter, ggconst.FileRouterGen), []byte(code), 0o600))
}
