package ggcheck_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
)

func TestModelSingularNamingAllowsExemptPlurals(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	// types, data and stats are plural in form but name one body of content,
	// so model directories and files may keep them; records, statistics and
	// metrics are ordinary plurals.
	for _, dir := range []string{"types", "data", "stats", "records", "statistics", "metrics"} {
		if err := os.MkdirAll(filepath.Join(projectDir, "model", dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeCheckFile(t, filepath.Join(projectDir, "model", "stats", "stats.go"), "package stats\n")

	violations := runCheck(ggcheck.ModelSingularNaming)

	if len(violations) != 3 {
		t.Fatalf("expected the ordinary plural model directory violations only, got %#v", violations)
	}
	for _, dir := range []string{"records", "statistics", "metrics"} {
		assertViolationContains(t, violations, filepath.Join("model", dir), "should be singular")
	}
}

func TestModelSingularNamingSkipsGitIgnoredPaths(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	// A runtime artifact directory ignored by Git rules, such as the log
	// directory a test run leaves behind, must not fail naming checks.
	writeCheckFile(t, filepath.Join(projectDir, ".gitignore"), "logs\n")
	if err := os.MkdirAll(filepath.Join(projectDir, "model", "user", "logs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "model", "records"), 0o755); err != nil {
		t.Fatal(err)
	}

	violations := runCheck(ggcheck.ModelSingularNaming)

	for _, violation := range violations {
		if strings.Contains(violation, filepath.Join("model", "user", "logs")) {
			t.Fatalf("git-ignored directory should be skipped, got violations: %#v", violations)
		}
	}
	if len(violations) != 1 || !strings.Contains(violations[0], filepath.Join("model", "records")) {
		t.Fatalf("expected only non-ignored plural directory violation, got %#v", violations)
	}
}
