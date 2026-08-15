package router_test

import (
	"testing"

	"github.com/hydroan/gst/router"
	"github.com/hydroan/gst/testutil"
)

// baseURL addresses the test server. The port is picked per test binary, so it
// resolves once here instead of at every call site.
var baseURL = testutil.BaseURL()

// TestMain brings up the server the way a generated project does: router.Init
// first, then the route registrations each test file contributes, which is the
// order generated code runs them in.
func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{
		Routes: func() error {
			if err := router.Init(); err != nil {
				return err
			}
			registerSSERoutes()
			registerDocumentedRoute()

			return nil
		},
	})
}
