package main

import "testing"

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
