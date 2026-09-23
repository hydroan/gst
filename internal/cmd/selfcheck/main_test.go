package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

// TestRun runs every check over the fixture module of TestCheckTestPlacement
// and gets what make check prints for it: the two test files that check
// reports there.
func TestRun(t *testing.T) {
	violations, err := run("testdata/testplacement/module")
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

func TestRunFailsOnAPackageThatDoesNotTypeCheck(t *testing.T) {
	_, err := run("testdata/broken")
	require.ErrorContains(t, err, "does not type-check")
}

// loadFixture loads the fixture module at dir the way run loads the tree it
// checks, and returns the module's absolute root with its packages.
func loadFixture(t *testing.T, dir string) (string, []*packages.Package) {
	t.Helper()
	root, err := filepath.Abs(dir)
	require.NoError(t, err)
	pkgs, err := load(root, nil, "./...")
	require.NoError(t, err)
	return root, pkgs
}
