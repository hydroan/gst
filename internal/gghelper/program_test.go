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

func TestProjectProgramPassesItsArguments(t *testing.T) {
	t.Chdir(t.TempDir())
	writeProjectFile(t, "go.mod", "module tmpapp\n\ngo 1.27\n")

	var out bytes.Buffer
	program := gghelper.ProjectProgram{
		Content: "package main\n\nimport (\n\t\"fmt\"\n\t\"os\"\n\t\"strings\"\n)\n\nfunc main() { fmt.Print(strings.Join(os.Args[1:], \",\")) }\n",
		Stdout:  &out,
		Args:    []string{"sample", "with space"},
	}
	require.NoError(t, program.Run())

	require.Equal(t, "sample,with space", out.String())
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

func TestProjectProgramRunsInsideAWorkspace(t *testing.T) {
	goMod := newWorkspaceProject(t)

	var out bytes.Buffer
	program := gghelper.ProjectProgram{
		Content: "package main\n\nimport (\n\t\"fmt\"\n\n\t\"example.com/lib\"\n)\n\nfunc main() { fmt.Print(libname.Source) }\n",
		Stdout:  &out,
	}
	require.NoError(t, program.Run())

	// The workspace, not a requirement of tmpapp, is what resolves lib, and
	// tmpapp's go.mod stays as it was.
	require.Equal(t, "workspace", out.String())
	content, err := os.ReadFile("go.mod")
	require.NoError(t, err)
	require.Equal(t, goMod, string(content))
}

func TestListProjectPackagesInsideAWorkspace(t *testing.T) {
	goMod := newWorkspaceProject(t)

	listed, err := gghelper.ListProjectPackages([]string{"example.com/lib", "math/rand/v2"})
	require.NoError(t, err)

	require.Equal(t, "libname", listed["example.com/lib"].Name)
	require.Equal(t, "rand", listed["math/rand/v2"].Name)
	content, err := os.ReadFile("go.mod")
	require.NoError(t, err)
	require.Equal(t, goMod, string(content))
}

// newWorkspaceProject lays out a workspace of two modules, tmpapp and
// example.com/lib, where tmpapp requires nothing and reaches lib through
// go.work alone, enters tmpapp and points GOWORK at the workspace file. It
// returns tmpapp's go.mod content, which the test expects to find unchanged.
func newWorkspaceProject(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	goMod := "module tmpapp\n\ngo 1.27\n"
	writeProjectFile(t, filepath.Join(dir, "app", "go.mod"), goMod)
	writeProjectFile(t, filepath.Join(dir, "lib", "go.mod"), "module example.com/lib\n\ngo 1.27\n")
	writeProjectFile(t, filepath.Join(dir, "lib", "lib.go"), "package libname\n\nconst Source = \"workspace\"\n")
	goWork := filepath.Join(dir, "go.work")
	writeProjectFile(t, goWork, "go 1.27\n\nuse (\n\t./app\n\t./lib\n)\n")
	t.Chdir(filepath.Join(dir, "app"))
	t.Setenv("GOWORK", goWork)
	return goMod
}
