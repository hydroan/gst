package new_test

import (
	"slices"
	"strings"
	"testing"

	pkgnew "github.com/hydroan/gst/internal/codegen/new"
	"github.com/hydroan/gst/internal/ggconst"
)

// TestProjectFilesListsTheScaffoldThenTheProjectFiles pins the order gg new
// writes a project in: the scaffold sorted by path, then main.go, .gitignore
// and config.ini.example, whose application is named after the last element
// of the module path.
func TestProjectFilesListsTheScaffoldThenTheProjectFiles(t *testing.T) {
	files, err := pkgnew.ProjectFiles("example.com/sampleapp")
	if err != nil {
		t.Fatal(err)
	}

	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	if len(paths) < 4 {
		t.Fatalf("paths = %v, want the scaffold and the three project files", paths)
	}
	scaffold, last := paths[:len(paths)-3], paths[len(paths)-3:]
	if !slices.IsSorted(scaffold) {
		t.Fatalf("scaffold paths = %v, want them sorted", scaffold)
	}
	if want := []string{ggconst.FileMain, ".gitignore", "config.ini.example"}; !slices.Equal(last, want) {
		t.Fatalf("last paths = %v, want %v", last, want)
	}
	for _, dir := range ggconst.ProjectImportDirs {
		if !slices.ContainsFunc(scaffold, func(path string) bool { return strings.HasPrefix(path, dir+"/") }) {
			t.Fatalf("scaffold lacks a file for the imported directory %q: %v", dir, scaffold)
		}
	}
	if config := files[len(files)-1].Content; !strings.Contains(config, "name = sampleapp\n") {
		t.Fatalf("config.ini.example does not name the application sampleapp:\n%s", config)
	}
	if main := files[len(files)-3].Content; !strings.Contains(main, "example.com/sampleapp/") {
		t.Fatalf("main.go does not import the project packages of example.com/sampleapp:\n%s", main)
	}
}
