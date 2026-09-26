package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gggen"
	"github.com/stretchr/testify/require"
)

// TestParseRouteTreeFromFileReadsGeneratedRoutes parses a router file built
// the way gg gen writes router/router.gen.go and finds every route it
// registers, with the HTTP method of each action: export and SSE routes are
// served over GET like the rest of the reads.
func TestParseRouteTreeFromFileReadsGeneratedRoutes(t *testing.T) {
	code, err := gggen.BuildRouterFile("router", "model", map[string]string{"tmpapp/model": ""},
		gggen.StmtRouterRegister("model", "Record", "*Record", "*Record", "model", "Auth", "/api/records", "", "Create"),
		gggen.StmtRouterRegister("model", "Record", "*Record", "*Record", "model", "Auth", "/api/records", "", "List"),
		gggen.StmtRouterRegister("model", "Record", "*Record", "*Record", "model", "Auth", "/api/records/:rec", "rec", "Get"),
		gggen.StmtRouterRegister("model", "Record", "*Record", "*Record", "model", "Pub", "/api/records/:rec", "rec", "Delete"),
		gggen.StmtRouterRegister("model", "Record", "*Record", "*Record", "model", "Auth", "/api/records/export", "", "Export"),
		gggen.StmtRouterRegister("model", "Notice", "*Notice", "*Notice", "model", "Auth", "/api/notices", "", "SSE"),
	)
	require.NoError(t, err)

	t.Chdir(t.TempDir())
	require.NoError(t, os.Mkdir(ggconst.DirModel, 0o755))
	require.NoError(t, os.Mkdir(ggconst.DirRouter, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ggconst.DirRouter, ggconst.FileRouterGen), []byte(code), 0o600))

	routes, err := parseRouteTreeFromFile()
	require.NoError(t, err)
	require.Equal(t, map[string][]string{
		"/api/records":        {"POST", "GET"},
		"/api/records/:rec":   {"GET", "DELETE"},
		"/api/records/export": {"GET"},
		"/api/notices":        {"GET"},
	}, routes)
}
