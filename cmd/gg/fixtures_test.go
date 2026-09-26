package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/gggen/columns"
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
// once per test binary, and errFrameworkSources keeps what stopped that
// recording for every later caller: go test keeps what a binary read for its
// whole run, and the framework does not change under a running test.
var (
	frameworkSourcesRecorded sync.Once
	errFrameworkSources      error
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
			errFrameworkSources = errors.Wrap(err, "list the framework packages")
			return
		}
		for line := range strings.Lines(string(output)) {
			fields := strings.Split(strings.TrimSuffix(line, "\n"), "\t")
			if _, err := os.Stat(fields[0]); err != nil {
				errFrameworkSources = err
				return
			}
			for _, name := range fields[1:] {
				if _, err := os.ReadFile(filepath.Join(fields[0], name)); err != nil {
					errFrameworkSources = err
					return
				}
			}
		}
	})
	if errFrameworkSources != nil {
		t.Fatal(errFrameworkSources)
	}
}

// childProjectEnv marks the process running one test in a project of its
// own; see newGenProject.
const childProjectEnv = "GG_TEST_PROJECT_CHILD"

// newGenProject gives a test that runs gg a fresh project, and a process of
// its own to run in. gg works in the current directory, the project root,
// and a test that changes the directory of its process cannot run in
// parallel with the others (see testing.T.Chdir); with dozens of tests each
// generating, compiling and inspecting a project, running them one after
// the other is what makes this package the slowest of the framework's. So
// the test binary runs each of these tests again in a child process of its
// own: in the parent, newGenProject starts the child (see runInChild),
// relays its verdict and returns ok false, on which the test returns at
// once; in the child, told apart by childProjectEnv, it creates the project,
// makes it the working directory and resets the gg command globals gen
// reads (module, prune), restoring them when the test ends, and returns the
// directory with ok true. The project's go.mod resolves the framework to
// this repository, since generation compiles the column inspection program
// against the framework for real. Generation also caches that inspection
// under the user cache directory, keyed by project directory; nothing ever
// reads a throwaway project's entry again, so the entry is removed when the
// test ends.
func newGenProject(t *testing.T) (projectDir string, ok bool) {
	t.Helper()
	if os.Getenv(childProjectEnv) == "" {
		runInChild(t)
		return "", false
	}

	oldModule := module
	oldPrune := prune
	t.Cleanup(func() {
		module = oldModule
		prune = oldPrune
	})

	projectDir = t.TempDir()
	t.Chdir(projectDir)
	module = ""
	prune = false

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
	return projectDir, true
}

// runInChild runs the calling test, its subtests included, in a child
// process of this test binary and relays the outcome: the test fails with
// the child's output when the child fails, each line marked so that go
// test's reading of the output never takes the child's own test lines for
// the parent's. The child runs this test alone, in a fresh directory, with
// the flags this run was given — a golden -update reaches it — but its own
// -test.run, no -test.v, and none of the files go test hands the parent to
// write, its input log and profiles among them: the child's would overwrite
// the parent's. The test runs in parallel with the other tests taking a
// project; the framework sources are recorded as the parent's inputs, the
// parent being the process go test caches the result of.
func runInChild(t *testing.T) {
	t.Helper()
	t.Parallel()
	recordFrameworkSources(t, frameworkRepoRoot(t))

	args := []string{"-test.run=" + childRunPattern(t.Name())}
	skipNext := false
	for _, arg := range os.Args[1:] {
		if skipNext {
			skipNext = false
			continue
		}
		flag, _, joined := strings.Cut(arg, "=")
		switch flag {
		case "-test.run", "-test.v", "-test.testlogfile", "-test.coverprofile", "-test.gocoverdir", "-test.outputdir",
			"-test.cpuprofile", "-test.memprofile", "-test.blockprofile", "-test.mutexprofile", "-test.trace":
			skipNext = !joined
			continue
		}
		args = append(args, arg)
	}
	cmd := exec.Command(os.Args[0], args...)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), childProjectEnv+"=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the test failed in its own process (%v):\n%s", err, childOutput(output))
	}
}

// childRunPattern returns the -test.run pattern selecting the test name and
// nothing else: every element anchored and quoted, "TestX/a_b" giving
// "^TestX$/^a_b$".
func childRunPattern(name string) string {
	elements := strings.Split(name, "/")
	for i, element := range elements {
		elements[i] = "^" + regexp.QuoteMeta(element) + "$"
	}
	return strings.Join(elements, "/")
}

// childOutput marks every line of a child's output, so that a "--- FAIL" or
// "=== RUN" line of the child reads as output of the parent's test to go
// test, not as a test line of its own.
func childOutput(output []byte) string {
	lines := strings.Split(strings.TrimRight(string(output), "\n"), "\n")
	for i, line := range lines {
		lines[i] = "| " + line
	}
	return strings.Join(lines, "\n")
}

// requireProjectCompiles type-checks every package of the project in the
// working directory, the generated sources and the handwritten code reading
// them alike. It runs go vet rather than go build: building links the
// project's main package against the whole framework, which costs seconds per
// project and proves nothing about the sources that type-checking them does
// not. -trimpath keeps the project's temporary directory out of what the build
// cache keys on, so a rerun reuses what the previous run compiled instead of
// storing another copy of it.
func requireProjectCompiles(t *testing.T) {
	t.Helper()

	output, err := exec.Command("go", "vet", "-trimpath", "-mod=mod", "./...").CombinedOutput()
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
