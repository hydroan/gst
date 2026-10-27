package ggnew

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/stretchr/testify/require"
)

// TestScaffoldCoversEveryImportedDirectory proves every package a generated
// main.go imports has a scaffold file: gg new creates it and gg gen restores
// it, so the import never dangles.
func TestScaffoldCoversEveryImportedDirectory(t *testing.T) {
	files, err := scaffoldFiles("helloworld")
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

// TestExamplesCarryTheScaffoldGitignore proves the example projects ignore
// what gg new writes, byte for byte: they stand in for a generated project,
// so a rule that changes here changes there, and gg gen and gg check read
// them through the same rules a generated project is read through.
func TestExamplesCarryTheScaffoldGitignore(t *testing.T) {
	for _, example := range []string{"demo", "cluster", "bench"} {
		t.Run(example, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join("..", "..", "examples", example, ".gitignore"))
			require.NoError(t, err)
			require.Equal(t, gitignoreContent, string(content))
		})
	}
}
