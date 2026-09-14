package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/codegen/constants"
)

// The migration program's model set is whatever the linked packages
// registered, so it has to link the project packages a generated main.go
// links and the framework entry point that links the rest, and it has to
// wait for module registration instead of guessing when it is done.
func TestMigrateProgramLinksWhatMainLinks(t *testing.T) {
	program := buildMigrateProgramForMode("sample", false, "")

	for _, dir := range constants.ProjectImportDirs {
		want := fmt.Sprintf("_ %q", "sample/"+dir)
		if !strings.Contains(program, want) {
			t.Fatalf("expected the migration program to import %s, got:\n%s", want, program)
		}
	}
	for _, want := range []string{`_ "github.com/hydroan/gst/bootstrap"`, "module.Wait()"} {
		if !strings.Contains(program, want) {
			t.Fatalf("expected the migration program to contain %s, got:\n%s", want, program)
		}
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "main.go", program, parser.AllErrors); err != nil {
		t.Fatalf("expected the migration program to parse, got %v", err)
	}
}
