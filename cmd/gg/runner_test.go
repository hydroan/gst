package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListProjectPackagesLeavesModuleFilesUntouched(t *testing.T) {
	t.Chdir(t.TempDir())
	goMod := "module tmpapp\n\ngo 1.27\n\nreplace example.com/dep => ./dep\n"
	writeCheckFile(t, "go.mod", goMod)
	writeCheckFile(t, filepath.Join("dep", "go.mod"), "module example.com/dep\n\ngo 1.27\n")
	writeCheckFile(t, filepath.Join("dep", "dep.go"), "package depname\n")

	listed, err := listProjectPackages([]string{"example.com/dep", "math/rand/v2"})
	require.NoError(t, err)

	// A package name need not match the last element of its path.
	require.Equal(t, "depname", listed["example.com/dep"].Name)
	require.Equal(t, "rand", listed["math/rand/v2"].Name)
	require.NotEmpty(t, listed["math/rand/v2"].GoFiles)
	// Resolving example.com/dep adds a requirement the project does not
	// declare yet, which must land in the copy of go.mod.
	content, err := os.ReadFile("go.mod")
	require.NoError(t, err)
	require.Equal(t, goMod, string(content))
}
