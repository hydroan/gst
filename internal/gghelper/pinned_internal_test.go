package gghelper

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/mod/modfile"
)

// TestPinnedCommandRunsTheProgramItBuiltApartFromTheProject pins how
// PinnedCommand runs a program: built into a directory of the user cache
// named after the module and version, from a go.mod there that requires
// that module at that version alone and gets its go.sum filled in beside
// it, then run in the caller's working directory with the caller's
// environment untouched; a later call finds the program up to date and
// leaves it, and once the go.sum is there a build needs no network. The
// protoc-gen-go of the protobuf module the framework requires stands in for
// the programs gg pins, so the module cache the framework's own build fills
// serves the test. It also pins the example of the pinnedModFile doc
// comment.
func TestPinnedCommandRunsTheProgramItBuiltApartFromTheProject(t *testing.T) {
	cache := t.TempDir()
	original := userCacheDir
	userCacheDir = func() (string, error) { return cache, nil }
	t.Cleanup(func() { userCacheDir = original })
	const module, pkg = "google.golang.org/protobuf", "google.golang.org/protobuf/cmd/protoc-gen-go"
	version := requiredVersion(t, module)

	cmd, err := PinnedCommand(module, version, pkg, "--version")
	require.NoError(t, err)

	dir := filepath.Join(cache, "gg", "run", module+"@"+version)
	program := filepath.Join(dir, "protoc-gen-go")
	require.Equal(t, []string{program, "--version"}, cmd.Args)
	require.Empty(t, cmd.Dir, "the program runs where the caller is, the project")
	require.Nil(t, cmd.Env, "the program runs with the caller's environment, untouched")
	out, err := cmd.Output()
	require.NoError(t, err)
	require.Equal(t, "protoc-gen-go "+version+"\n", string(out))

	goMod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	require.NoError(t, err)
	toolchain := strings.TrimPrefix(runtime.Version(), "go")
	require.Equal(t, "module gg.run\n\ngo "+toolchain+"\n\nrequire "+module+" "+version+"\n", string(goMod))
	goSum, err := os.ReadFile(filepath.Join(dir, "go.sum"))
	require.NoError(t, err)
	require.Contains(t, string(goSum), module+" "+version+" h1:")
	require.Equal(t, "module gg.run\n\ngo 1.27.1\n\nrequire google.golang.org/protobuf v1.36.12\n", pinnedModFile("google.golang.org/protobuf", "v1.36.12", "go1.27.1"))

	// A later call finds the program up to date and leaves it (the go
	// command touches its modification time and nothing else), and the
	// go.sum with it.
	built, err := os.Stat(program)
	require.NoError(t, err)
	_, err = PinnedCommand(module, version, pkg, "--version")
	require.NoError(t, err)
	again, err := os.Stat(program)
	require.NoError(t, err)
	require.True(t, os.SameFile(built, again), "the program was written again")
	goSumAgain, err := os.ReadFile(filepath.Join(dir, "go.sum"))
	require.NoError(t, err)
	require.Equal(t, string(goSum), string(goSumAgain))

	// With the go.sum written, a build reads the module cache alone: the
	// program builds again with the module proxy switched off.
	require.NoError(t, os.Remove(program))
	t.Setenv("GOPROXY", "off")
	cmd, err = PinnedCommand(module, version, pkg, "--version")
	require.NoError(t, err)
	out, err = cmd.Output()
	require.NoError(t, err)
	require.Equal(t, "protoc-gen-go "+version+"\n", string(out))
}

// TestPinnedCommandReportsABuildThatFails pins that a package the module
// does not hold fails the call, naming the package, and returns no command.
// The module proxy is switched off so that the go command does not go
// looking for another module providing the package.
func TestPinnedCommandReportsABuildThatFails(t *testing.T) {
	cache := t.TempDir()
	original := userCacheDir
	userCacheDir = func() (string, error) { return cache, nil }
	t.Cleanup(func() { userCacheDir = original })
	t.Setenv("GOPROXY", "off")
	const module = "google.golang.org/protobuf"
	version := requiredVersion(t, module)

	cmd, err := PinnedCommand(module, version, module+"/cmd/no-such-program")

	require.Nil(t, cmd)
	require.ErrorContains(t, err, "build "+module+"/cmd/no-such-program@"+version)
}

// requiredVersion returns the version the framework's go.mod requires of
// module.
func requiredVersion(t *testing.T, module string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	require.NoError(t, err)
	file, err := modfile.Parse("go.mod", data, nil)
	require.NoError(t, err)
	for _, req := range file.Require {
		if req.Mod.Path == module {
			return req.Mod.Version
		}
	}
	t.Fatalf("go.mod requires no %s", module)
	return ""
}
