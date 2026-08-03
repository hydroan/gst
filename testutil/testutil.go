// Package testutil is the test suite a gst project writes its tests against.
//
// A test package hands Run everything it needs and gets a running application
// back: the backing services come up in containers of their own, the framework
// bootstraps against them, the server starts serving, and all of it is released
// once the tests are done. Nothing has to be installed on the machine running
// the tests beyond a container runtime.
//
//	var baseURL = testutil.BaseURL()
//
//	func TestMain(m *testing.M) {
//		testutil.Run(m, testutil.Server{
//			Database: config.DBMySQL,
//			Redis:    true,
//			Routes:   func() { util.RunOrDie(router.Init) },
//		})
//	}
//
//	func TestPing(t *testing.T) {
//		cli, err := client.New(baseURL)
//		require.NoError(t, err)
//
//		rsp, err := client.Get[model.PingRsp](cli, "/api/pings")
//		require.NoError(t, err)
//		require.NotNil(t, rsp)
//	}
//
// A login is a plain client.Post: the client's cookie jar holds the session
// cookie, so every later request through the same client is authenticated.
// Rejections are asserted with RequireError.
//
// This package forwards to the framework-internal implementation and adds no
// behavior of its own.
package testutil

import (
	"testing"

	"github.com/hydroan/gst/client"
	testutil "github.com/hydroan/gst/internal/testutil"
)

// Server declares what a test package needs before its tests can run.
type Server = testutil.Server

// Run prepares what s declares, starts the test server, runs the tests and
// releases everything afterwards. It is the whole body of a test package's
// TestMain and does not return: it exits with the result of the tests.
func Run(m *testing.M, s Server) {
	testutil.Run(m, s)
}

// BaseURL returns the test server base address clients are constructed with.
func BaseURL() string {
	return testutil.BaseURL()
}

// URL returns an absolute URL of the test server for path. The port is picked
// per test binary, so an endpoint can be declared as a package-level variable.
func URL(path string) string {
	return testutil.URL(path)
}

// DecodeResp asserts that resp carries a successful envelope and returns the
// decoded payload.
func DecodeResp[RSP any](t *testing.T, resp *client.Envelope) RSP {
	t.Helper()

	return testutil.DecodeResp[RSP](t, resp)
}

// RequireError asserts that err is a server-side rejection with the given
// HTTP status code and that the business message contains every msgContains
// entry. The rejection is returned by value for follow-up asserts, such as
// the business code.
func RequireError(t *testing.T, err error, statusCode int, msgContains ...string) client.Error {
	t.Helper()

	return testutil.RequireError(t, err, statusCode, msgContains...)
}

// RequireDataFields asserts that the raw envelope data carries every named
// top-level JSON field, guarding serialization contracts against fields
// silently vanishing behind omitempty.
func RequireDataFields(t *testing.T, resp *client.Envelope, fields ...string) {
	t.Helper()

	testutil.RequireDataFields(t, resp, fields...)
}
