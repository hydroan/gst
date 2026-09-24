//nolint:predeclared
package new

import (
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/stretchr/testify/require"
)

// TestScaffoldCoversEveryImportedDirectory proves every package a generated
// main.go imports has a scaffold file: gg new creates it and gg gen restores
// it, so the import never dangles.
func TestScaffoldCoversEveryImportedDirectory(t *testing.T) {
	files, err := scaffoldFiles()
	require.NoError(t, err)
	for _, dir := range ggconst.ProjectImportDirs {
		found := false
		for path := range files {
			if strings.HasPrefix(path, dir+"/") && strings.HasSuffix(path, ".go") {
				found = true
				break
			}
		}
		require.Truef(t, found, "directory %q is imported by main.go but has no scaffold file", dir)
	}
}
