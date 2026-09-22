package gghelper_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/gghelper"
)

func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "record.go")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if !gghelper.FileExists(file) {
		t.Fatalf("FileExists(%q) = false, want true for a file", file)
	}
	if !gghelper.FileExists(dir) {
		t.Fatalf("FileExists(%q) = false, want true for a directory", dir)
	}
	if missing := filepath.Join(dir, "missing.go"); gghelper.FileExists(missing) {
		t.Fatalf("FileExists(%q) = true, want false", missing)
	}
}

// TestEnsureParentDir pins the example of EnsureParentDir: writing
// model/sample/record.go needs model/sample, which is created when missing and
// left alone when present.
func TestEnsureParentDir(t *testing.T) {
	t.Chdir(t.TempDir())
	path := filepath.Join("model", "sample", "record.go")

	if err := gghelper.EnsureParentDir(path); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join("model", "sample")); err != nil || !info.IsDir() {
		t.Fatalf("model/sample after EnsureParentDir: %v, want a directory", err)
	}
	if err := gghelper.EnsureParentDir(path); err != nil {
		t.Fatalf("EnsureParentDir() on an existing parent: %v", err)
	}
}

// TestRelativePath pins that RelativePath turns a path under the working
// directory into a relative one and hands a relative path back unchanged.
func TestRelativePath(t *testing.T) {
	t.Chdir(t.TempDir())
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	rel := filepath.Join("model", "record.go")

	if got := gghelper.RelativePath(filepath.Join(cwd, rel)); got != rel {
		t.Fatalf("RelativePath(absolute) = %q, want %q", got, rel)
	}
	if got := gghelper.RelativePath(rel); got != rel {
		t.Fatalf("RelativePath(%q) = %q, want it unchanged", rel, got)
	}
}
