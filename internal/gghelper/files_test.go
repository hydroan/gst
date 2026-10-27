package gghelper_test

import (
	"os"
	"path/filepath"
	"slices"
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

// TestPathUnderRoot pins the examples of PathUnderRoot: a path under the root
// comes back cleaned, and one climbing out of it is refused.
func TestPathUnderRoot(t *testing.T) {
	under := []struct{ path, want string }{
		{"service/record/create.go", "service/record/create.go"},
		{"service/./record/create.go", "service/record/create.go"},
		{"service", "service"},
	}
	for _, tt := range under {
		got, err := gghelper.PathUnderRoot(filepath.FromSlash(tt.path), "service")
		if want := filepath.FromSlash(tt.want); err != nil || got != want {
			t.Fatalf("PathUnderRoot(%q, service) = %q, %v; want %q", tt.path, got, err, want)
		}
	}
	for _, path := range []string{"service/../main.go", "../service/record.go", "model/record.go"} {
		if got, err := gghelper.PathUnderRoot(filepath.FromSlash(path), "service"); err == nil {
			t.Fatalf("PathUnderRoot(%q, service) = %q, want an error", path, got)
		}
	}
}

// TestPairedTestFilesListsTheTestsBesideAServiceFile pins the pairing rule
// prune and module copy share: the external and the internal test file
// beside a service file, in that order, only those on disk, and no test file
// of another stem.
func TestPairedTestFilesListsTheTestsBesideAServiceFile(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"create.go", "create_test.go", "create_internal_test.go", "list.go", "helper_test.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("package sample\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if got, want := gghelper.PairedTestFiles(filepath.Join(dir, "create.go")), []string{filepath.Join(dir, "create_test.go"), filepath.Join(dir, "create_internal_test.go")}; !slices.Equal(got, want) {
		t.Fatalf("PairedTestFiles(create.go) = %v, want %v", got, want)
	}
	if got := gghelper.PairedTestFiles(filepath.Join(dir, "list.go")); len(got) != 0 {
		t.Fatalf("PairedTestFiles(list.go) = %v, want none", got)
	}
}
