package main

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/hydroan/gst/router"
)

func TestRoutePhaseMethodMatchesRuntimeRegistration(t *testing.T) {
	// Expected methods mirror the runtime registration table in
	// router/router.go (Register): Export is served via GET, Import via POST.
	tests := []struct {
		phase string
		want  string
	}{
		{"Create", "POST"},
		{"CreateMany", "POST"},
		{"Import", "POST"},
		{"Delete", "DELETE"},
		{"DeleteMany", "DELETE"},
		{"Update", "PUT"},
		{"UpdateMany", "PUT"},
		{"Patch", "PATCH"},
		{"PatchMany", "PATCH"},
		{"List", "GET"},
		{"Get", "GET"},
		{"Export", "GET"},
		{"Unknown", ""},
	}
	for _, tt := range tests {
		if got := routePhaseMethod(tt.phase); got != tt.want {
			t.Errorf("routePhaseMethod(%q) = %q, want %q", tt.phase, got, tt.want)
		}
	}
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
	want := "base: " + router.APIPathPrefix
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
