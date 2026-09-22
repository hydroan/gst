package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/stretchr/testify/require"
)

// TestParseRouteTreeFromFileReadsGeneratedRoutes parses a router file built
// the way gg gen writes router/router.gen.go and finds every route it
// registers, with the HTTP method of each action: export and SSE routes are
// served over GET like the rest of the reads.
func TestParseRouteTreeFromFileReadsGeneratedRoutes(t *testing.T) {
	code, err := gen.BuildRouterFile("router", "model", map[string]string{"tmpapp/model": ""},
		gen.StmtRouterRegister("model", "Record", "*Record", "*Record", "model", "Auth", "records", "", "Create"),
		gen.StmtRouterRegister("model", "Record", "*Record", "*Record", "model", "Auth", "records", "", "List"),
		gen.StmtRouterRegister("model", "Record", "*Record", "*Record", "model", "Auth", "records/:rec", "rec", "Get"),
		gen.StmtRouterRegister("model", "Record", "*Record", "*Record", "model", "Pub", "records/:rec", "rec", "Delete"),
		gen.StmtRouterRegister("model", "Record", "*Record", "*Record", "model", "Auth", "records/export", "", "Export"),
		gen.StmtRouterRegister("model", "Notice", "*Notice", "*Notice", "model", "Auth", "notices", "", "SSE"),
	)
	require.NoError(t, err)

	t.Chdir(t.TempDir())
	require.NoError(t, os.Mkdir(ggconst.DirModel, 0o755))
	require.NoError(t, os.Mkdir(ggconst.DirRouter, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ggconst.DirRouter, ggconst.FileRouterGen), []byte(code), 0o600))

	routes, err := parseRouteTreeFromFile()
	require.NoError(t, err)
	require.Equal(t, map[string][]string{
		"records":        {"POST", "GET"},
		"records/:rec":   {"GET", "DELETE"},
		"records/export": {"GET"},
		"notices":        {"GET"},
	}, routes)
}
