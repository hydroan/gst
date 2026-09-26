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
func TestWritebackFilesTheResultUnderTheLinkKey(t *testing.T) {
	tool := buildTool(t)
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

	require.NoError(t, writeback([]string{record}))

	out = goTest(t, module, "-x", "./pkg/")
	require.Contains(t, out, "(cached)")
	require.Equal(t, 0, linksIn(out), "the written-back result is found by the link key")
}

// buildTool builds the command under test into a temporary directory.
func buildTool(t *testing.T) string {
	t.Helper()
	tool := filepath.Join(t.TempDir(), "buildcache")
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
	"testing"

	"example.com/orphan/dep"
)

func TestUsed(t *testing.T) {
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
