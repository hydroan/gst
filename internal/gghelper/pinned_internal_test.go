package gghelper

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/mod/modfile"
)

// TestPinnedCommandRunsTheProgramItBuiltApartFromTheProject pins how
// PinnedCommand runs a program: built into a directory of the user cache
// named after the module and version, from a go.mod there that requires
// that module at that version alone, carries the go directive of the go
// command at hand and gets its go.sum filled in beside it, then run in the
// caller's working directory with the caller's environment untouched; a
// later call finds the program up to date and leaves it, and once the
// go.sum is there a build needs no network. The protoc-gen-go of the
// protobuf module the framework requires stands in for the programs gg
// pins, so the module cache the framework's own build fills serves the
// test. It also pins the example of the pinnedModFile doc comment.
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
	toolchain := strings.TrimPrefix(goVersion(t), "go")
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

// TestPinnedCommandBuildsWithTheGoCommandAtHand pins that the go directive
// of the pinned go.mod names the go command PinnedCommand runs, not the
// toolchain gg was built with: a go on PATH that reports an older version
// gets a go.mod it satisfies, so the build runs on it instead of fetching
// a newer toolchain. The go on PATH is a script that answers go env
// GOVERSION with a version one step below the real one and hands every
// other command to the real go.
func TestPinnedCommandBuildsWithTheGoCommandAtHand(t *testing.T) {
	cache := t.TempDir()
	original := userCacheDir
	userCacheDir = func() (string, error) { return cache, nil }
	t.Cleanup(func() { userCacheDir = original })
	const module, pkg = "google.golang.org/protobuf", "google.golang.org/protobuf/cmd/protoc-gen-go"
	version := requiredVersion(t, module)
	real, err := exec.LookPath("go")
	require.NoError(t, err)
	older := olderGoVersion(t, goVersion(t))
	bin := t.TempDir()
	script := "#!/bin/sh\nif [ \"$1\" = env ] && [ \"$2\" = GOVERSION ]; then echo " + older + "; exit 0; fi\nexec \"" + real + "\" \"$@\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "go"), []byte(script), 0o700)) // #nosec G306 -- the script stands in for the go command and must be executable
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	cmd, err := PinnedCommand(module, version, pkg, "--version")
	require.NoError(t, err)

	goMod, err := os.ReadFile(filepath.Join(cache, "gg", "run", module+"@"+version, "go.mod"))
	require.NoError(t, err)
	require.Contains(t, string(goMod), "\ngo "+strings.TrimPrefix(older, "go")+"\n")
	out, err := cmd.Output()
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

// goVersion returns what go env GOVERSION reports for the go on PATH.
func goVersion(t *testing.T) string {
	t.Helper()

	out, err := exec.Command("go", "env", "GOVERSION").Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

// releaseVersion reads a release toolchain version, go1.27.1 or go1.27,
// into its parts.
var releaseVersion = regexp.MustCompile(`^go(\d+)\.(\d+)(?:\.(\d+))?$`)

// olderGoVersion returns a toolchain version one step below toolchain: the
// minor without its patch for go1.27.1 (go1.27, the same as go1.27.0), the
// previous minor for go1.27 (go1.26). A development toolchain names no
// release, so the test skips.
func olderGoVersion(t *testing.T, toolchain string) string {
	t.Helper()

	m := releaseVersion.FindStringSubmatch(toolchain)
	if m == nil {
		t.Skipf("%s is no release toolchain", toolchain)
	}
	if m[3] != "" && m[3] != "0" {
		return "go" + m[1] + "." + m[2]
	}
	minor, err := strconv.Atoi(m[2])
	require.NoError(t, err)
	require.Positive(t, minor)
	return "go" + m[1] + "." + strconv.Itoa(minor-1)
}
