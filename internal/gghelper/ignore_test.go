package gghelper_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/gghelper"
)

// TestProjectIgnoreWalkPrunesIgnoredPaths pins what the shared walk hands its
// caller: the root itself even when a rule names it, every kept path, and
// nothing a Git ignore rule or the go command leaves out, an excluded
// directory being pruned whole instead of walked.
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
	write("go.mod", "module example.com/app\n\ngo 1.27\n\nignore ./model/assets\nignore node_modules\n")
	write(".gitignore", "model/scratch.go\nmodel/tmp/\n")
	write(filepath.Join("model", "record.go"), "package model\n")
	write(filepath.Join("model", "scratch.go"), "package model\n")
	write(filepath.Join("model", "tmp", "draft.go"), "package tmp\n")
	write(filepath.Join("model", "_draft.go"), "package model\n")
	write(filepath.Join("model", "_old", "old.go"), "package old\n")
	write(filepath.Join("model", ".cache", "cached.go"), "package cached\n")
	write(filepath.Join("model", "vendor", "lib", "lib.go"), "package lib\n")
	write(filepath.Join("model", "testdata", "fixture.go"), "package fixture\n")
	write(filepath.Join("model", "nested", "go.mod"), "module example.com/nested\n")
	write(filepath.Join("model", "nested", "nested.go"), "package nested\n")
	write(filepath.Join("model", "assets", "asset.go"), "package assets\n")
	write(filepath.Join("model", "sample", "node_modules", "lib", "lib.go"), "package lib\n")

	var seen []string
	err := gghelper.NewProjectIgnore().Walk("model", func(path string, info os.FileInfo) error {
		seen = append(seen, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"model", filepath.Join("model", "record.go"), filepath.Join("model", "sample")}
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

// TestProjectIgnoreIgnoresBelowWhatTheGoCommandLeavesOut pins that Ignores
// judges every directory of a path, as a caller asking about one file has not
// walked there: a file below a directory the go command leaves out is
// ignored, one below a kept directory is not.
func TestProjectIgnoreIgnoresBelowWhatTheGoCommandLeavesOut(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("go.mod", []byte("module example.com/app\n\ngo 1.27\n\nignore ./web\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ignore := gghelper.NewProjectIgnore()

	tests := []struct {
		path string
		want bool
	}{
		{path: filepath.Join("model", "sample", "item.go")},
		{path: filepath.Join("model", "_draft", "item.go"), want: true},
		{path: filepath.Join("model", "testdata", "item.go"), want: true},
		{path: filepath.Join("web", "assets", "app.go"), want: true},
	}
	for _, tt := range tests {
		if got := ignore.Ignores(tt.path, false); got != tt.want {
			t.Errorf("Ignores(%q) = %t, want %t", tt.path, got, tt.want)
		}
	}
}

// TestProjectIgnoreExcludedByGo pins the paths the go command leaves out of a
// ./... pattern, the rules of cmd/go one by one, including the ignore
// directives of the project's go.mod: ignore ./web names web alone, ignore
// node_modules every node_modules directory. A path is judged by its own name
// and what it holds, so an absolute path is judged as the project path it is.
func TestProjectIgnoreExcludedByGo(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, dir := range []string{
		".git", "vendor", "tools", "web", filepath.Join("gomod", "go.mod"),
		filepath.Join("service", "sample"), filepath.Join("service", ".cache"), filepath.Join("service", "testdata"),
		filepath.Join("service", "_draft"), filepath.Join("service", "web"), filepath.Join("service", "node_modules"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		"go.mod":                         "module example.com/app\n\ngo 1.27\n\nignore ./web\nignore node_modules\n",
		filepath.Join("tools", "go.mod"): "module example.com/tools\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	ignore := gghelper.NewProjectIgnore()

	tests := []struct {
		name  string
		path  string
		isDir bool
		want  bool
	}{
		{name: "the project root", path: ".", isDir: true},
		{name: "a project directory", path: "service", isDir: true},
		{name: "a directory below it", path: filepath.Join("service", "sample"), isDir: true},
		{name: "a Go file", path: filepath.Join("service", "sample.go")},
		{name: "a hidden directory", path: ".git", isDir: true, want: true},
		{name: "a hidden directory further down", path: filepath.Join("service", ".cache"), isDir: true, want: true},
		{name: "a hidden file", path: filepath.Join("service", ".sample.go"), want: true},
		{name: "a directory named with a leading underscore", path: filepath.Join("service", "_draft"), isDir: true, want: true},
		{name: "a file named with a leading underscore", path: filepath.Join("service", "_draft.go"), want: true},
		{name: "vendor", path: "vendor", isDir: true, want: true},
		{name: "testdata further down", path: filepath.Join("service", "testdata"), isDir: true, want: true},
		{name: "a file named testdata", path: filepath.Join("service", "testdata")},
		{name: "a directory holding its own go.mod", path: "tools", isDir: true, want: true},
		{name: "a directory holding a directory named go.mod", path: "gomod", isDir: true},
		{name: "a directory the go.mod ignores from the module root", path: "web", isDir: true, want: true},
		{name: "the same name further down", path: filepath.Join("service", "web"), isDir: true},
		{name: "a directory the go.mod ignores anywhere", path: filepath.Join("service", "node_modules"), isDir: true, want: true},
		{name: "an absolute path the go.mod ignores", path: filepath.Join(wd, "web"), isDir: true, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ignore.ExcludedByGo(tt.path, tt.isDir); got != tt.want {
				t.Fatalf("ExcludedByGo(%q, %t) = %t, want %t", tt.path, tt.isDir, got, tt.want)
			}
		})
	}
}
