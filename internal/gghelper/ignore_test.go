package gghelper_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/gghelper"
)

// TestProjectIgnoreWalkPrunesIgnoredPaths pins what the shared walk hands its
// caller: the root itself even when a rule names it, every kept path, and
// nothing under an ignored directory, which is pruned whole instead of walked.
func TestProjectIgnoreWalkPrunesIgnoredPaths(t *testing.T) {
	t.Chdir(t.TempDir())
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(".gitignore", "model/scratch.go\nmodel/tmp/\n")
	write(filepath.Join("model", "record.go"), "package model\n")
	write(filepath.Join("model", "scratch.go"), "package model\n")
	write(filepath.Join("model", "tmp", "draft.go"), "package tmp\n")

	var seen []string
	err := gghelper.NewProjectIgnore().Walk("model", func(path string, info os.FileInfo) error {
		seen = append(seen, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"model", filepath.Join("model", "record.go")}
	if len(seen) != len(want) {
		t.Fatalf("walked %q, want %q", seen, want)
	}
	for i, path := range want {
		if seen[i] != path {
			t.Fatalf("walked %q, want %q", seen, want)
		}
	}
}

// TestProjectIgnoreWalkFailsOnAMissingRoot pins that a walk error aborts
// instead of reaching the callback: the callback only ever sees paths that
// exist.
func TestProjectIgnoreWalkFailsOnAMissingRoot(t *testing.T) {
	t.Chdir(t.TempDir())

	called := false
	err := gghelper.NewProjectIgnore().Walk("model", func(path string, info os.FileInfo) error {
		called = true
		return nil
	})

	if err == nil {
		t.Fatal("Walk() over a missing root returned no error")
	}
	if called {
		t.Fatal("Walk() called the callback for a path that does not exist")
	}
}

// TestProjectIgnoreIgnoresWithoutRules pins that a project without ignore
// rules ignores nothing.
func TestProjectIgnoreIgnoresWithoutRules(t *testing.T) {
	t.Chdir(t.TempDir())

	if gghelper.NewProjectIgnore().Ignores(filepath.Join("model", "record.go"), false) {
		t.Fatal("Ignores() = true without any ignore rule")
	}
}
