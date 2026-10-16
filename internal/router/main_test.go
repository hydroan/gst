package router_test

import (
	"os"
	"testing"

	"github.com/hydroan/gst/internal/testutil"
)

// TestMain brings up the server the way a generated project does: the
// bootstrap builds the engine and its route groups, then the route
// registrations each test file contributes run on them, which is where
// generated code runs its own.
func TestMain(m *testing.M) {
	// A network no test connects from is the trusted proxy: a request a test
	// hands the engine as arriving from it is believed to forward the
	// client, while the loopback the real connections come from speaks for
	// itself (see clientip_test.go and clientip_internal_test.go).
	_ = os.Setenv("SERVER_TRUSTED_PROXIES", "10.0.0.0/8")
	testutil.Run(m, testutil.Server{
		Routes: func() error {
			registerSSERoutes()
			registerDocumentedRoute()
			registerClientIPRoute()

			return nil
		},
	})
}
