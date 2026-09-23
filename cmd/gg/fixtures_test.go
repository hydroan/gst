package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/codegen/gen/columns"
	"github.com/stretchr/testify/require"
)

func writeProjectFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// sourceLine returns the 1-based number of the first line of source that
// contains fragment, so a test can name the line a report points at without
// restating its number.
func sourceLine(t *testing.T, source string, fragment string) int {
	t.Helper()

	index := slices.IndexFunc(strings.Split(source, "\n"), func(line string) bool {
		return strings.Contains(line, fragment)
	})
	if index < 0 {
		t.Fatalf("no line of the source contains %q", fragment)
	}
	return index + 1
}

// writeProjectGoMod writes the fixture project's go.mod together with the
// minimal framework source it resolves through the go module graph: checks
// that exempt copied framework modules resolve the framework source for every
// project they inspect.
func writeProjectGoMod(t *testing.T, projectDir string) {
	t.Helper()

	writeProjectFile(t, filepath.Join(projectDir, "go.mod"), "module tmpapp\n\ngo 1.26\n\nrequire github.com/hydroan/gst v0.0.0-00010101000000-000000000000\n\nreplace github.com/hydroan/gst => ./internal/gst\n")
	writeProjectFile(t, filepath.Join(projectDir, "internal", "gst", "go.mod"), "module github.com/hydroan/gst\n\ngo 1.26\n")
	if err := os.MkdirAll(filepath.Join(projectDir, "internal", "gst", "module"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// writeProjectGoModAgainstRealFramework writes the fixture project's
// go.mod resolving the framework to this repository's own source tree. Gen
// fixtures need it because the generated inspection program compiles against
// the framework for real, which a stub source tree cannot satisfy. The
// fixture reuses this repository's own go.mod requirements and go.sum so the
// build resolves the exact dependency versions the framework pins, instead of
// re-resolving the graph from scratch (which trips over ambiguous-import
// splits such as google.golang.org/genproto).
func writeProjectGoModAgainstRealFramework(t *testing.T, projectDir string) {
	t.Helper()

	root := frameworkRepoRoot(t)
	goMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	content := strings.Replace(string(goMod), "module github.com/hydroan/gst\n", "module tmpapp\n", 1)
	content += "\nrequire github.com/hydroan/gst v0.0.0-00010101000000-000000000000\n\nreplace github.com/hydroan/gst => " + root + "\n"
	writeProjectFile(t, filepath.Join(projectDir, "go.mod"), content)
	goSum, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	writeProjectFile(t, filepath.Join(projectDir, "go.sum"), string(goSum))
	recordFrameworkSources(t, root)
}

// frameworkSourcesRecorded makes recordFrameworkSources record the sources
// once per test binary: go test keeps what a binary read for its whole run,
// and the framework does not change under a running test.
var (
	frameworkSourcesRecorded sync.Once
	frameworkSourcesErr      error
)

// recordFrameworkSources records the framework's Go sources under root as
// inputs of the test. The programs these fixtures build compile against those
// sources in a child go command, which go test does not see as an input of
// the test; recording them here does, so a change to the framework reruns the
// test instead of replaying a cached result that no longer holds.
//
// What gets recorded is what a build of the framework depends on and nothing
// else: every package directory is stat'ed, which notices a file added to it
// or removed from it, and every file the package compiles or embeds is read.
// No directory is listed. go test hashes a listed directory by the name, size
// and modification time of every entry, so listing the repository root would
// take every git operation, which touches .git, for a framework change and
// cost the package its test cache. The packages come from go list in a child
// process, whose reads go test does not record.
func recordFrameworkSources(t *testing.T, root string) {
	t.Helper()

	frameworkSourcesRecorded.Do(func() {
		list := exec.Command("go", "list", "-e", "-f",
			`{{.Dir}}{{range .GoFiles}}{{"\t"}}{{.}}{{end}}{{range .CgoFiles}}{{"\t"}}{{.}}{{end}}{{range .EmbedFiles}}{{"\t"}}{{.}}{{end}}`,
			"./...")
		list.Dir = root
		output, err := list.Output()
		if err != nil {
			frameworkSourcesErr = errors.Wrap(err, "list the framework packages")
			return
		}
		for line := range strings.Lines(string(output)) {
			fields := strings.Split(strings.TrimSuffix(line, "\n"), "\t")
			if _, err := os.Stat(fields[0]); err != nil {
				frameworkSourcesErr = err
				return
			}
			for _, name := range fields[1:] {
				if _, err := os.ReadFile(filepath.Join(fields[0], name)); err != nil {
					frameworkSourcesErr = err
					return
				}
			}
		}
	})
	if frameworkSourcesErr != nil {
		t.Fatal(frameworkSourcesErr)
	}
}

// newGenProject creates a temporary project for tests that run gg gen, makes
// it the working directory and resets the gg command globals gen reads
// (module, prune, cleanOrphans), restoring them when the test ends. Its go.mod resolves
// the framework to this repository, since generation compiles the column
// inspection program against the framework for real. Generation also caches
// that inspection under the user cache directory, keyed by project directory;
// nothing ever reads a throwaway project's entry again, so the entry is removed
// when the test ends.
func newGenProject(t *testing.T) string {
	t.Helper()

	oldModule := module
	oldPrune := prune
	oldCleanOrphans := cleanOrphans
	t.Cleanup(func() {
		module = oldModule
		prune = oldPrune
		cleanOrphans = oldCleanOrphans
	})

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	module = ""
	prune = false
	cleanOrphans = false

	writeProjectGoModAgainstRealFramework(t, projectDir)
	cacheDir, err := columns.CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if removeErr := os.RemoveAll(cacheDir); removeErr != nil {
			t.Error(removeErr)
		}
	})
	return projectDir
}

// requireProjectCompiles type-checks every package of the project in the
// working directory, the generated sources and the handwritten code reading
// them alike. It runs go vet rather than go build: building links the
// project's main package against the whole framework, which costs seconds per
// project and proves nothing about the sources that type-checking them does
// not.
func requireProjectCompiles(t *testing.T) {
	t.Helper()

	output, err := exec.Command("go", "vet", "-mod=mod", "./...").CombinedOutput()
	require.NoError(t, err, "go vet:\n%s", output)
}

// frameworkRepoRoot returns the absolute path of this repository's root.
func frameworkRepoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}
