package gghelper_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/gghelper"
	"github.com/stretchr/testify/require"
)

func TestProjectProgramRunsAgainstTheProjectModuleWithOverlay(t *testing.T) {
	t.Chdir(t.TempDir())
	goMod := "module tmpapp\n\ngo 1.27\n"
	onDisk := "package sample\n\nconst Source = \"disk\"\n"
	writeProjectFile(t, "go.mod", goMod)
	writeProjectFile(t, filepath.Join("sample", "sample.go"), onDisk)

	var out bytes.Buffer
	program := gghelper.ProjectProgram{
		Content: "package main\n\nimport (\n\t\"fmt\"\n\n\t\"tmpapp/sample\"\n)\n\nfunc main() { fmt.Print(sample.Source) }\n",
		Stdout:  &out,
		Overlay: map[string]string{filepath.Join("sample", "sample.go"): "package sample\n\nconst Source = \"overlay\"\n"},
	}
	require.NoError(t, program.Run())

	// The program imports the project's own package and compiles it with the
	// overlay's replacement, while the files on disk stay as they were.
	require.Equal(t, "overlay", out.String())
	content, err := os.ReadFile(filepath.Join("sample", "sample.go"))
	require.NoError(t, err)
	require.Equal(t, onDisk, string(content))
	content, err = os.ReadFile("go.mod")
	require.NoError(t, err)
	require.Equal(t, goMod, string(content))
}

func TestListProjectPackagesLeavesModuleFilesUntouched(t *testing.T) {
	t.Chdir(t.TempDir())
	goMod := "module tmpapp\n\ngo 1.27\n\nreplace example.com/dep => ./dep\n"
	writeProjectFile(t, "go.mod", goMod)
	writeProjectFile(t, filepath.Join("dep", "go.mod"), "module example.com/dep\n\ngo 1.27\n")
	writeProjectFile(t, filepath.Join("dep", "dep.go"), "package depname\n")

	listed, err := gghelper.ListProjectPackages([]string{"example.com/dep", "math/rand/v2"})
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

// writeProjectFile writes content to path, creating its parent directories.
func writeProjectFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
