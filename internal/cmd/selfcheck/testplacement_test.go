package main

import (
	"sort"
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
	root, pkgs := loadFixture(t, "testdata/testplacement/module")
	violations, err := checkTestPlacement(root, pkgs)
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

// TestExternalCandidates pins the static pass on its own, over the fixture
// module of TestCheckTestPlacement. checkTestPlacement confirms every
// candidate by type-checking it, so a static rule gone wrong would not change
// what checkTestPlacement reports; it would only send more files to that far
// slower confirmation, and the static pass is what keeps the check fast on a
// tree with nothing to report. Only two files get past it: the one
// checkTestPlacement reports (movable) and the one only the confirmation stops
// (sealed).
func TestExternalCandidates(t *testing.T) {
	root, pkgs := loadFixture(t, "testdata/testplacement/module")

	var got []string
	for _, p := range pkgs {
		if !isInternalTestVariant(p) || p.Name == "main" {
			continue
		}
		for _, f := range externalCandidates(analyzeTestFiles(p)) {
			got = append(got, relative(root, f.path))
		}
	}
	sort.Strings(got)
	require.Equal(t, []string{"movable/movable_internal_test.go", "sealed/sealed_internal_test.go"}, got)
}
