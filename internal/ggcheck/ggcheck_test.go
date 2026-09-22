package ggcheck_test

import (
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
)

// TestRunReturnsOneResultPerCheckInOrder pins that Run answers with one result
// per check, in the order the checks were given, each under its check's name.
func TestRunReturnsOneResultPerCheckInOrder(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record-item.go"), "package record\n")

	results := ggcheck.Run([]ggcheck.Check{ggcheck.ModelFileBoundaries, ggcheck.ModelFileNameHyphens})

	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].Name != "Model file boundaries" || len(results[0].Violations) != 0 {
		t.Fatalf("results[0] = %+v, want a clean Model file boundaries result", results[0])
	}
	if results[1].Name != "Model file name hyphens" || len(results[1].Violations) != 1 {
		t.Fatalf("results[1] = %+v, want one Model file name hyphens violation", results[1])
	}
}
