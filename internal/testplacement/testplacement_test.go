package testplacement_test

import (
	"testing"

	"github.com/hydroan/gst/internal/testplacement"
	"github.com/stretchr/testify/require"
)

// TestCheck runs the check over a fixture module with one package per case.
// Two test files are named as internal tests without having to be: one
// declares the external test package, and one uses nothing unexported. Every
// other internal test needs to be one for a reason the check has to
// recognize, one per package: an unexported name (clean), a method declared on
// the package's type (method), an unkeyed literal of a struct with an
// unexported field (unkeyed), a fixture file an internal test uses (fixture),
// a helper that ties a test to such a file (tied), and an interface only the
// package can implement, which only type-checking the move shows (sealed). A
// main package is left alone (cmd/tool).
func TestCheck(t *testing.T) {
	violations, err := testplacement.Check("testdata/module")
	require.NoError(t, err)
	require.Equal(t, []testplacement.Violation{
		{
			File:    "mislabeled/mislabeled_internal_test.go",
			Message: "Test file 'mislabeled/mislabeled_internal_test.go' is named as an internal test but declares package mislabeled_test: drop _internal from its name",
		},
		{
			File:    "movable/movable_internal_test.go",
			Message: "Test file 'movable/movable_internal_test.go' uses nothing unexported of package movable: declare package movable_test and drop _internal from its name",
		},
	}, violations)
}

func TestCheckFailsOnAPackageThatDoesNotTypeCheck(t *testing.T) {
	_, err := testplacement.Check("testdata/broken")
	require.ErrorContains(t, err, "does not type-check")
}
