package ggcheck_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
	"github.com/hydroan/gst/internal/modelinspect"
)

// TestGeneratedFilesNamesTheFilesGenWouldWrite pins the check on a project
// compiled against the framework: with a model and nothing generated it
// names every file gg gen would create — main.go, the four registration
// files and the column reference file of the model — a file holding
// something else is one gg gen would rewrite, and model code that does not
// compile leaves the column reference files unchecked, said so, while the
// files rendered from the declarations alone are still compared. What a run
// of gg gen leaves the check to report, nothing, is pinned beside the run,
// in cmd/gg.
func TestGeneratedFilesNamesTheFilesGenWouldWrite(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	// The check caches the column inspection under the user cache directory,
	// keyed by project directory; nothing reads a throwaway project's entry
	// again, so it goes when the test ends.
	cacheDir, err := modelinspect.CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if removeErr := os.RemoveAll(cacheDir); removeErr != nil {
			t.Error(removeErr)
		}
	})
	writeCheckProjectGoModAgainstRealFramework(t, projectDir)
	record := `package sample

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	Title string ` + "`json:\"title\"`" + `

	model.Base
}

func (Record) Design() {
	dsl.Migrate()
	dsl.Endpoint("records")
	dsl.Create(func() {})
}
`
	// The inspection imports the root model package, which a project has from
	// the day gg new wrote it.
	writeCheckFile(t, filepath.Join(projectDir, "model", "model.go"), "package model\n")
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "record.go"), record)

	created := func(path string) string { return path + ": gg gen would create it; run gg gen" }
	want := []string{
		created("main.go"),
		created(filepath.Join("model", "apidoc.gen.go")),
		created(filepath.Join("model", "model.gen.go")),
		created(filepath.Join("model", "sample", "record.gen.go")),
		created(filepath.Join("router", "router.gen.go")),
		created(filepath.Join("service", "service.gen.go")),
	}
	if violations := runCheck(ggcheck.GeneratedFiles); !slices.Equal(violations, want) {
		t.Fatalf("violations = %#v, want every file gg gen would create, by path", violations)
	}

	// A file holding anything but what gg gen writes is one it would rewrite.
	writeCheckFile(t, filepath.Join(projectDir, "router", "router.gen.go"), "package router\n")
	want[4] = filepath.Join("router", "router.gen.go") + ": gg gen would rewrite it; run gg gen"
	if violations := runCheck(ggcheck.GeneratedFiles); !slices.Equal(violations, want) {
		t.Fatalf("violations = %#v, want the router file reported as one gg gen would rewrite", violations)
	}

	// Model code the compiler refuses cannot be asked for its columns; the
	// registration files and main.go come from the declarations alone.
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "broken.go"), "package sample\n\nvar broken int = \"text\"\n")
	violations := runCheck(ggcheck.GeneratedFiles)
	if len(violations) != 6 {
		t.Fatalf("violations = %#v, want the five files compared and the column reference files unchecked", violations)
	}
	assertViolationContains(t, violations, "column reference files", "the column reference files could not be checked: ")
	assertViolationContains(t, violations, filepath.Join("model", "model.gen.go"), "gg gen would create it")
}
