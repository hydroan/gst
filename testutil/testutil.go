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
// Database state is asserted with the Require* family, which reads rows back
// through the framework query chain and fails the test when the state does
// not match. Writing rows is not its job: tests seed and mutate data through
// the database chain or their own seed helpers.
//
// This package forwards to the framework-internal implementation and adds no
// behavior of its own.
package testutil

import (
	"testing"

	"github.com/hydroan/gst/client"
	testutil "github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/types"
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

// RequireGet asserts that the row with the given id exists and returns it.
func RequireGet[T any, M interface {
	types.Model
	*T
}](t *testing.T, id string) M {
	t.Helper()

	return testutil.RequireGet[T, M](t, id)
}

// RequireFirst asserts that at least one row matches the query and returns
// the first match. A nil or zero-value query matches no rows: WithQuery keeps
// the framework's empty-query safety check, so a deliberate full-table read
// stays on the database chain.
func RequireFirst[T any, M interface {
	types.Model
	*T
}](t *testing.T, query M) M {
	t.Helper()

	return testutil.RequireFirst[T, M](t, query)
}

// RequireList asserts that listing the rows matching the query succeeds and
// returns them, sorted by orders when given. An empty result is a valid
// return; length assertions stay with the caller. A nil or zero-value query
// matches no rows: WithQuery keeps the framework's empty-query safety check,
// so a deliberate full-table read stays on the database chain.
func RequireList[T any, M interface {
	types.Model
	*T
}](t *testing.T, query M, orders ...types.Order) []M {
	t.Helper()

	return testutil.RequireList[T, M](t, query, orders...)
}

// RequireNoRow asserts that no row with the given id is visible through the
// regular query path. A hard-deleted and a soft-deleted row both satisfy it;
// RequireSoftDeleted additionally pins that a soft-deleted row is kept.
func RequireNoRow[T any, M interface {
	types.Model
	*T
}](t *testing.T, id string) {
	t.Helper()

	testutil.RequireNoRow[T, M](t, id)
}

// RequireSoftDeleted asserts that the row with the given id is gone from the
// regular query path while its record is kept with the soft-delete column
// set.
func RequireSoftDeleted[T any, M interface {
	types.Model
	*T
}](t *testing.T, id string) {
	t.Helper()

	testutil.RequireSoftDeleted[T, M](t, id)
}

// SwapValue sets *field to value for the duration of the test and restores
// the previous value on cleanup, the way t.Setenv does for environment
// variables. It swaps process-wide state such as bootstrapped configuration
// fields, so tests touching the same field must not run in parallel.
func SwapValue[T any](t *testing.T, field *T, value T) {
	t.Helper()

	testutil.SwapValue(t, field, value)
}

// DownloadCSV downloads path as a CSV export through cli and parses the
// attachment into records, stripping a leading UTF-8 BOM if present. The
// helper sets the CSV format parameter itself; opts carry the remaining
// query parameters, such as filters.
func DownloadCSV(t *testing.T, cli *client.Client, path string, opts ...client.RequestOption) [][]string {
	t.Helper()

	return testutil.DownloadCSV(t, cli, path, opts...)
}
