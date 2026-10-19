package main

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWritebackFilesTheResultUnderTheLinkKey pins the whole mechanism on the
// toolchain at hand. A change the linker drops from a test binary leaves the
// binary linked again on every cached run, since go files the result only
// under the binary's content; after the write-back the run links nothing.
// The steps that assert go's own behavior are what tells a toolchain that
// writes the result back itself: the tool is then done for.
//
// The test reads two environment variables go computes, one go sets in its
// own process and one it does not, so the write-back has to hash them the
// way go did. It runs against a cache of its own: the machine's is left as
// it is.
func TestWritebackFilesTheResultUnderTheLinkKey(t *testing.T) {
	tool := buildTool(t)
	t.Setenv("GOCACHE", t.TempDir())
	module := writeOrphanModule(t)
	t.Chdir(module)

	// A first run runs the test and files its result under both keys.
	record := t.TempDir()
	out := goTest(t, module, "-toolexec", tool+" record-link "+record, "./pkg/")
	require.NotContains(t, out, "(cached)")
	records, err := readRecords(record)
	require.NoError(t, err)
	require.Len(t, records, 1, "the test binary was linked once")
	require.Equal(t, "example.com/orphan/pkg", records[0].importPath)

	// A second run finds the result by the link key and links nothing.
	record = t.TempDir()
	out = goTest(t, module, "-toolexec", tool+" record-link "+record, "./pkg/")
	require.Contains(t, out, "(cached)")
	_, err = os.Stat(filepath.Join(record, recordFile))
	require.ErrorIs(t, err, fs.ErrNotExist, "a result found by the link key skips the link")

	// A function the test never reaches changes the link key and not the
	// binary: the binary is linked, the result is found by its content, and
	// go files nothing under the new link key, so the next run links again.
	writeFile(t, filepath.Join(module, "dep", "unused.go"), "package dep\n\nfunc Unused() int { return 2 }\n")
	record = t.TempDir()
	out = goTest(t, module, "-toolexec", tool+" record-link "+record, "./pkg/")
	require.Contains(t, out, "(cached)")
	records, err = readRecords(record)
	require.NoError(t, err)
	require.Len(t, records, 1, "the orphaned binary was linked again")
	require.Equal(t, 1, linksIn(goTest(t, module, "-x", "./pkg/")), "and is linked on every run until the result is filed under the link key")

	cmd := exec.Command(tool, "writeback", record)
	cmd.Dir = module
	out2, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out2)

	out = goTest(t, module, "-x", "./pkg/")
	require.Contains(t, out, "(cached)")
	require.Equal(t, 0, linksIn(out), "the written-back result is found by the link key")
}

// TestGoEnvLeavesOutWhatGoReportsWithoutSetting pins the environment the
// write-back aligns with go's: what go computes and sets in its process is
// in, what it computes for the report alone is out.
func TestGoEnvLeavesOutWhatGoReportsWithoutSetting(t *testing.T) {
	env, err := goEnv()
	require.NoError(t, err)

	for _, name := range []string{"GOARCH", "GOCACHE", "GOPROXY", "GOROOT"} {
		require.Contains(t, env, name)
	}
	for _, name := range []string{"GOMOD", "GOWORK", "GOBIN", "CGO_CFLAGS", "GOGCCFLAGS"} {
		require.NotContains(t, env, name)
	}
}

// buildTool builds the command under test into a temporary directory.
func buildTool(t *testing.T) string {
	t.Helper()
	tool := filepath.Join(t.TempDir(), "testcachefix")
	cmd := exec.Command("go", "build", "-o", tool, ".")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	return tool
}

// writeOrphanModule writes a module whose package pkg tests a function of
// package dep, the shape an orphaned test binary needs: a change to dep
// that pkg's test does not reach.
func writeOrphanModule(t *testing.T) string {
	t.Helper()
	module := t.TempDir()
	writeFile(t, filepath.Join(module, "go.mod"), "module example.com/orphan\n\ngo 1.27\n")
	writeFile(t, filepath.Join(module, "dep", "dep.go"), "package dep\n\nfunc Used() int { return 1 }\n")
	writeFile(t, filepath.Join(module, "pkg", "pkg.go"), "package pkg\n")
	writeFile(t, filepath.Join(module, "pkg", "pkg_test.go"), `package pkg

import (
	"os"
	"testing"

	"example.com/orphan/dep"
)

func TestUsed(t *testing.T) {
	// GOPROXY is computed by go and set in its own process, GOMOD computed
	// and reported only; a test that reads either puts it in its input list.
	t.Setenv("GOPROXY", "off")
	_ = os.Getenv("GOMOD")
	if dep.Used() != 1 {
		t.Fatal("dep.Used() != 1")
	}
}
`)
	return module
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

// goTest runs go test -json in the module with args and returns its JSON
// stream, the form make test consumes and keys results on.
func goTest(t *testing.T, module string, args ...string) string {
	t.Helper()
	cmd := exec.Command("go", append([]string{"test", "-json"}, args...)...)
	cmd.Dir = module
	out, err := cmd.Output()
	require.NoError(t, err, "%s", out)
	return string(out)
}

// linksIn counts the test binaries a go test -x -json stream linked.
func linksIn(out string) int {
	return strings.Count(out, "link -o ")
}
