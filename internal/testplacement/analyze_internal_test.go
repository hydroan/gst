package testplacement

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCandidates pins the static pass on its own, over the fixture module of
// TestCheck. Check confirms every candidate by type-checking it, so a static
// rule gone wrong would not change what Check reports; it would only send more
// files to that far slower confirmation, and the static pass is what keeps
// the check fast on a tree with nothing to report. Only two files get past it:
// the one Check reports (movable) and the one only the confirmation stops
// (sealed).
func TestCandidates(t *testing.T) {
	root, err := filepath.Abs("testdata/module")
	require.NoError(t, err)
	pkgs, err := load(root, nil, "./...")
	require.NoError(t, err)

	var got []string
	for _, p := range pkgs {
		if !isInternalTestVariant(p) || p.Name == "main" {
			continue
		}
		for _, f := range candidates(analyze(p)) {
			got = append(got, relative(root, f.path))
		}
	}
	sort.Strings(got)
	require.Equal(t, []string{"movable/movable_internal_test.go", "sealed/sealed_internal_test.go"}, got)
}
