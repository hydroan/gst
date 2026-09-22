package ggcheck_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
)

func TestDirectoryRestrictionsAcceptsConventionalProjectDirectories(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "go.mod"), "module tmpapp\n\ngo 1.26\n\nrequire github.com/hydroan/gst v0.0.0\n")
	for _, dir := range []string{"deploy", "scripts", "test", "hack", "charts", "sample"} {
		if err := os.MkdirAll(filepath.Join(projectDir, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	violations := runCheck(ggcheck.DirectoryRestrictions)
	want := []string{"Directory 'sample' is not allowed in project structure"}
	if !slices.Equal(violations, want) {
		t.Fatalf("expected only the unplaced directory reported, got %#v", violations)
	}
}
