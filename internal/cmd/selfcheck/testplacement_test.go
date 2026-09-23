package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCheckTestPlacement runs the check over a fixture module with one package
// per case. Two test files are named as internal tests without having to be:
// one declares the external test package, and one uses nothing unexported.
// Every other internal test needs to be one for a reason the check has to
// recognize, one per package: an unexported name (clean), a method declared on
// the package's type (method), an unkeyed literal of a struct with an
// unexported field (unkeyed), a fixture file an internal test uses (fixture),
// a helper that ties a test to such a file (tied), and an interface only the
// package can implement, which only type-checking the move shows (sealed). A
// main package is left alone (cmd/tool).
func TestCheckTestPlacement(t *testing.T) {
	violations, err := checkTestPlacement("testdata/testplacement/module")
	require.NoError(t, err)
	require.Equal(t, []violation{
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

func TestCheckTestPlacementFailsOnAPackageThatDoesNotTypeCheck(t *testing.T) {
	_, err := checkTestPlacement("testdata/testplacement/broken")
	require.ErrorContains(t, err, "does not type-check")
}
