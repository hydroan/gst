package ggcheck_test

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
)

// runCheck runs check over the project in the working directory the way gg
// check runs it and returns the violations it finds.
func runCheck(check ggcheck.Check) []string {
	return ggcheck.Run([]ggcheck.Check{check})[0].Violations
}

func writeCheckFile(t *testing.T, path string, content string) {
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

// writeCheckProjectGoMod writes the fixture project's go.mod together with the
// minimal framework source it resolves through the go module graph: checks
// that exempt copied framework modules resolve the framework source for every
// project they inspect.
func writeCheckProjectGoMod(t *testing.T, projectDir string) {
	t.Helper()

	writeCheckFile(t, filepath.Join(projectDir, "go.mod"), "module tmpapp\n\ngo 1.26\n\nrequire github.com/hydroan/gst v0.0.0-00010101000000-000000000000\n\nreplace github.com/hydroan/gst => ./internal/gst\n")
	writeCheckFile(t, filepath.Join(projectDir, "internal", "gst", "go.mod"), "module github.com/hydroan/gst\n\ngo 1.26\n")
	if err := os.MkdirAll(filepath.Join(projectDir, "internal", "gst", "module"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// writeFrameworkModuleFixture writes a minimal framework source tree under
// projectDir/internal/gst declaring one copyable module, which is how
// CopyableModuleNames discovers module-owned service subtrees.
func writeFrameworkModuleFixture(t *testing.T, projectDir, name string) {
	t.Helper()

	frameworkDir := filepath.Join(projectDir, "internal", "gst")
	writeCheckFile(t, filepath.Join(frameworkDir, "go.mod"), "module github.com/hydroan/gst\n\ngo 1.26\n")
	writeCheckFile(t, filepath.Join(frameworkDir, "module", name, "register.go"), "package "+name+"\n\nfunc Register() {}\n")
	writeCheckFile(t, filepath.Join(frameworkDir, "module", name, "module.json"), "{}\n")
}

// assertViolationContains asserts that exactly one violation mentions path and
// that this violation also carries want.
func assertViolationContains(t *testing.T, violations []string, path, want string) {
	t.Helper()

	matched := make([]string, 0, 1)
	for _, violation := range violations {
		if strings.Contains(violation, path) {
			matched = append(matched, violation)
		}
	}
	if len(matched) != 1 {
		t.Fatalf("expected one violation for %s, got %#v", path, violations)
	}
	if !strings.Contains(matched[0], want) {
		t.Fatalf("expected violation for %s to contain %q, got %q", path, want, matched[0])
	}
}

// writeCheckProjectGoModAgainstRealFramework writes the fixture project's
// go.mod and go.sum so that its packages compile against this repository:
// the requirements and sums of the framework's own go.mod, the module
// renamed, and a replace pointing at the repository root. A check that
// derives the protobuf definitions loads the project's packages for real,
// which the stub source tree of writeCheckProjectGoMod cannot satisfy. The
// framework sources that load reads in a child go command are not inputs of
// the test; the generator's own tests, in cmd/gg, record them.
func writeCheckProjectGoModAgainstRealFramework(t *testing.T, projectDir string) {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("the path of this test file is unknown")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	goMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	content := strings.Replace(string(goMod), "module github.com/hydroan/gst\n", "module tmpapp\n", 1)
	content += "\nrequire github.com/hydroan/gst v0.0.0-00010101000000-000000000000\n\nreplace github.com/hydroan/gst => " + root + "\n"
	writeCheckFile(t, filepath.Join(projectDir, "go.mod"), content)
	goSum, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	writeCheckFile(t, filepath.Join(projectDir, "go.sum"), string(goSum))
}
