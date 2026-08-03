// Package testutil is the test suite a gst project writes its tests against.
//
// A test package hands Run everything it needs and gets a running application
// back: the backing services come up in containers of their own, the framework
// bootstraps against them, the server starts serving, and all of it is released
// once the tests are done. Nothing has to be installed on the machine running
// the tests beyond a container runtime.
//
//	var pingAPI = testutil.URL("/api/ping")
//
//	func TestMain(m *testing.M) {
//		testutil.Run(m, testutil.Server{
//			Database: config.DBMySQL,
//			Redis:    true,
//			Seed:     func() { util.RunOrDie(router.Init) },
//		})
//	}
//
// This package forwards to the framework-internal implementation and adds no
// behavior of its own.
package testutil

import (
	"testing"

	"github.com/hydroan/gst/client"
	testutil "github.com/hydroan/gst/internal/testutil"
)

type (
	// Server declares what a test package needs before its tests can run.
	Server = testutil.Server

	// ListResponse is the envelope a list endpoint answers with.
	ListResponse[T any] = testutil.ListResponse[T]
)

// Run prepares what s declares, starts the test server, runs the tests and
// releases everything afterwards. It is the whole body of a test package's
// TestMain and does not return: it exits with the result of the tests.
func Run(m *testing.M, s Server) {
	testutil.Run(m, s)
}

// URL returns an absolute URL of the test server for path. The port is picked
// per test binary, so an endpoint can be declared as a package-level variable.
func URL(path string) string {
	return testutil.URL(path)
}

// RequireResp asserts that resp carries a successful envelope and hands the
// decoded payload to checkFn.
func RequireResp[RSP any](t *testing.T, resp *client.Resp, checkFn func(t *testing.T, rsp RSP)) {
	t.Helper()

	testutil.RequireResp(t, resp, checkFn)
}
