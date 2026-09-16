// Package swap replaces process-wide values for the duration of one test.
//
// It depends on nothing but the testing package, so the tests of every
// framework package can use it, including the packages the test suite itself
// is built on.
package swap

import "testing"

// serialEnv is the environment variable Value sets to keep a test out of
// parallel runs.
const serialEnv = "GST_TEST_PROCESS_STATE_SWAPPED"

// Value sets *field to value for the duration of the test and restores the
// previous value on cleanup, the way t.Setenv does for environment variables.
//
// The field is process-wide state, such as a bootstrapped configuration field
// or a package variable of a dependency, so no other test may run while it is
// swapped. Value enforces that rather than leaving it to a comment: it sets an
// environment variable through t.Setenv, and the testing package refuses to
// combine t.Setenv with t.Parallel in either order. A swap in a test that
// already called t.Parallel panics, and so does a later t.Parallel call.
func Value[T any](t *testing.T, field *T, value T) {
	t.Helper()

	t.Setenv(serialEnv, "1")
	previous := *field
	*field = value
	t.Cleanup(func() { *field = previous })
}
