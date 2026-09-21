package router_test

import (
	"testing"

	"github.com/hydroan/gst/internal/testutil"
)

// TestMain brings up the server the way a generated project does: the
// bootstrap builds the engine and its route groups, then the route
// registrations each test file contributes run on them, which is where
// generated code runs its own.
func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{
		Routes: func() error {
			registerSSERoutes()
			registerDocumentedRoute()
			registerClientIPRoute()

			return nil
		},
	})
}
