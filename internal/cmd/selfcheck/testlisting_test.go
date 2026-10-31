package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCheckTestListing runs the check over a fixture module whose one package
// lists directories from its tests. Reported are the listings of
// TestListsElsewhere, which leave the package (the parent directory, a path
// below it, an absolute pattern, the parent of the package's own directory,
// an os.DirFS of the parent) or come from where the check cannot follow them
// (an environment variable), and the one in listAnywhere, which a caller
// hands a path outside the package. Left alone are those of TestListsInside,
// the package's own directory, its testdata, t.TempDir(), os.MkdirTemp, the
// directory of the file runtime.Caller reports and paths below it, the
// elements of a literal a range runs over and the parameter of listInside,
// which every caller names inside the package, and the listing in
// listing.go, which no test runs. A path concatenated from parts, in
// TestListsByConcatenation, is reported when a part spelt out climbs out of
// the package and left alone when the parts stay below it.
func TestCheckTestListing(t *testing.T) {
	root, pkgs := loadFixture(t, "testdata/testlisting/module")
	violations, err := checkTestListing(root, pkgs)
	require.NoError(t, err)

	reported := []struct {
		line int
		call string
	}{
		{31, "os.ReadDir"},
		{32, "filepath.WalkDir"},
		{33, "os.ReadDir"},
		{34, "filepath.Glob"},
		{35, "os.ReadDir"},
		{36, "os.DirFS"},
		{50, "os.ReadDir"},
		{65, "os.ReadDir"},
	}
	want := make([]violation, 0, len(reported))
	for _, r := range reported {
		want = append(want, violation{
			File: "listing/listing_test.go",
			Message: fmt.Sprintf("Test file 'listing/listing_test.go:%d': %s lists a directory the check cannot place inside the package or under t.TempDir(): "+
				"go test keys the cached result on every entry of a listed directory, so a listing elsewhere reruns the test whenever anything there moves; "+
				"read the files of another package through golist.Packages of internal/testutil/golist, which lists no directory", r.line, r.call),
		})
	}
	require.Equal(t, want, violations)
}
