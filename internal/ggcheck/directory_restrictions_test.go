package ggcheck_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
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

// TestDirectoryRestrictionsSkipsProjectsNotRequiringTheFramework pins that the
// layout is only held against projects that depend on the framework: a module
// whose path merely starts like the framework's is another project.
func TestDirectoryRestrictionsSkipsProjectsNotRequiringTheFramework(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "go.mod"), "module tmpapp\n\ngo 1.26\n\nrequire github.com/hydroan/gstx v0.0.0\n")
	if err := os.MkdirAll(filepath.Join(projectDir, "sample"), 0o755); err != nil {
		t.Fatal(err)
	}

	if violations := runCheck(ggcheck.DirectoryRestrictions); len(violations) != 0 {
		t.Fatalf("expected no violations for a project outside the framework, got %#v", violations)
	}
}

// TestDirectoryRestrictionsReportsAGoModItCannotParse pins that a go.mod the
// check cannot read is reported instead of skipping the layout silently.
func TestDirectoryRestrictionsReportsAGoModItCannotParse(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "go.mod"), "module tmpapp\n\ngo 1.26\n\nrequire github.com/hydroan/gst\n")
	if err := os.MkdirAll(filepath.Join(projectDir, "sample"), 0o755); err != nil {
		t.Fatal(err)
	}

	violations := runCheck(ggcheck.DirectoryRestrictions)
	if len(violations) != 1 || !strings.Contains(violations[0], "failed to parse go.mod") {
		t.Fatalf("expected the unparsable go.mod reported, got %#v", violations)
	}
}
