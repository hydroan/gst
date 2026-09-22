package ggcheck_test

import (
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
)

func TestModelFileNameHyphens(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record-item.go"), "package record\n")
	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record_note.go"), "package record\n")

	violations := runCheck(ggcheck.ModelFileNameHyphens)

	if len(violations) != 1 {
		t.Fatalf("expected one hyphenated model file violation, got %#v", violations)
	}
	assertViolationContains(t, violations, filepath.Join("model", "record", "record-item.go"), "should not contain hyphens (suggested: record_item.go)")
}
