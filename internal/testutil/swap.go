package testutil

import "testing"

// SwapValue sets *field to value for the duration of the test and restores
// the previous value on cleanup, the way t.Setenv does for environment
// variables. It swaps process-wide state such as bootstrapped configuration
// fields, so tests touching the same field must not run in parallel.
func SwapValue[T any](t *testing.T, field *T, value T) {
	t.Helper()

	previous := *field
	*field = value
	t.Cleanup(func() { *field = previous })
}
