package gghelper

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPinnedCommandRunsInAModuleOfItsOwn pins the shape of the command
// PinnedCommand returns and the module file it reads: a go run of the
// package in the caller's working directory reading, through -modfile, a
// go.mod in a directory of the user cache named after the module and
// version, which requires that module at that version alone, with -mod=mod
// added to GOFLAGS so the go command fills the go.sum in, and the go.mod
// written once and left alone afterwards. It also pins the example of the
// pinnedModFile doc comment.
func TestPinnedCommandRunsInAModuleOfItsOwn(t *testing.T) {
	cache := t.TempDir()
	original := userCacheDir
	userCacheDir = func() (string, error) { return cache, nil }
	t.Cleanup(func() { userCacheDir = original })
	t.Setenv("GOFLAGS", "-trimpath")

	cmd, err := PinnedCommand("example.com/pinned", "v1.2.3", "example.com/pinned/cmd/tool", "--version")
	require.NoError(t, err)

	dir := filepath.Join(cache, "gg", "run", "example.com/pinned@v1.2.3")
	require.Empty(t, cmd.Dir, "the program runs where the caller is, the project")
	require.Equal(t, []string{"go", "run", "-modfile", filepath.Join(dir, "go.mod"), "example.com/pinned/cmd/tool", "--version"}, cmd.Args)
	require.Contains(t, cmd.Env, "GOFLAGS=-trimpath -mod=mod")
	require.Contains(t, cmd.Env, "GOWORK=off")

	goMod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	require.NoError(t, err)
	version := strings.TrimPrefix(runtime.Version(), "go")
	require.Equal(t, "module gg.run\n\ngo "+version+"\n\nrequire example.com/pinned v1.2.3\n", string(goMod))
	require.Equal(t, "module gg.run\n\ngo 1.27.1\n\nrequire google.golang.org/protobuf v1.36.12\n", pinnedModFile("google.golang.org/protobuf", "v1.36.12", "go1.27.1"))

	// A later call keeps the file the first one wrote, along with the
	// go.sum the go command adds beside it.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.sum"), []byte("kept"), 0o600))
	_, err = PinnedCommand("example.com/pinned", "v1.2.3", "example.com/pinned/cmd/tool")
	require.NoError(t, err)
	goSum, err := os.ReadFile(filepath.Join(dir, "go.sum"))
	require.NoError(t, err)
	require.Equal(t, "kept", string(goSum))
}
