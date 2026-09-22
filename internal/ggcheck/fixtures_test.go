package ggcheck_test

import (
	"os"
	"path/filepath"
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
