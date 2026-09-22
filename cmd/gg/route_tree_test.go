package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/codegen/constants"
	"github.com/hydroan/gst/internal/codegen/gen"
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

	oldRouterDir, oldModelDir := routerDir, modelDir
	t.Cleanup(func() { routerDir, modelDir = oldRouterDir, oldModelDir })
	routerDir, modelDir = t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(routerDir, constants.FileRouterGen), []byte(code), 0o600))

	routes, err := parseRouteTreeFromFile()
	require.NoError(t, err)
	require.Equal(t, map[string][]string{
		"records":        {"POST", "GET"},
		"records/:rec":   {"GET", "DELETE"},
		"records/export": {"GET"},
		"notices":        {"GET"},
	}, routes)
}
