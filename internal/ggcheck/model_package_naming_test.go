package ggcheck_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
)

func TestModelPackageNamingAllowsUnderscoreStrippedAndExternalTestPackages(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	// A package name with underscores stripped from the directory name is allowed.
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample_record", "sample_record.go"), "package samplerecord\n")

	// A black-box test file using the `<package>_test` package name is allowed.
	writeCheckFile(t, filepath.Join(projectDir, "model", "group", "group.go"), "package group\n")
	writeCheckFile(t, filepath.Join(projectDir, "model", "group", "sample_record_test.go"), "package group_test\n")

	// A genuine mismatch between package name and directory name (after stripping underscores) should still be reported.
	writeCheckFile(t, filepath.Join(projectDir, "model", "mismatch", "mismatch.go"), "package wrongname\n")

	violations := runCheck(ggcheck.ModelPackageNaming)

	for _, violation := range violations {
		if strings.Contains(violation, filepath.Join("sample_record", "sample_record.go")) {
			t.Fatalf("underscore-stripped package name should be allowed, got violations: %#v", violations)
		}
		if strings.Contains(violation, filepath.Join("group", "sample_record_test.go")) {
			t.Fatalf("external test package name should be allowed, got violations: %#v", violations)
		}
	}
	if len(violations) != 1 || !strings.Contains(violations[0], filepath.Join("mismatch", "mismatch.go")) {
		t.Fatalf("expected only genuine package name mismatch violation, got %#v", violations)
	}
}

func TestModelPackageNamingSkipsGitIgnoredPaths(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeCheckFile(t, filepath.Join(projectDir, ".gitignore"), "generated\n")

	// A mismatched package inside a git-ignored directory must not be reported.
	writeCheckFile(t, filepath.Join(projectDir, "model", "user", "generated", "helper.go"), "package mismatched\n")

	// A genuine mismatch outside ignored paths should still be reported.
	writeCheckFile(t, filepath.Join(projectDir, "model", "mismatch", "mismatch.go"), "package wrongname\n")

	violations := runCheck(ggcheck.ModelPackageNaming)

	for _, violation := range violations {
		if strings.Contains(violation, "helper.go") {
			t.Fatalf("git-ignored path should be skipped, got violations: %#v", violations)
		}
	}
	if len(violations) != 1 || !strings.Contains(violations[0], filepath.Join("mismatch", "mismatch.go")) {
		t.Fatalf("expected only genuine package name mismatch violation, got %#v", violations)
	}
}
