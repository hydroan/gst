package main

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

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
