package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/codegen/constants"
)

// The migration program's model set is whatever the linked packages
// registered, so it has to link the project packages a generated main.go
// links and the framework entry point that links the rest, and it has to
// wait for module registration instead of guessing when it is done. A
// scaffold package the project lacks — restored by gg gen, never by migrate
// — is left out rather than failing the build, which is what an upgraded
// project that has not run gen yet looks like.
func TestMigrateProgramLinksWhatMainLinks(t *testing.T) {
	projectDir := t.TempDir()
	for _, dir := range constants.ProjectImportDirs {
		if dir == constants.SubDirLock {
			// Left missing on purpose.
			continue
		}
		if err := os.MkdirAll(filepath.Join(projectDir, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(projectDir, dir, dir+".go"), []byte("package "+dir+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(projectDir)

	program := buildMigrateProgramForMode("sample", false, "")

	for _, dir := range constants.ProjectImportDirs {
		want := fmt.Sprintf("_ %q", "sample/"+dir)
		if dir == constants.SubDirLock {
			if strings.Contains(program, want) {
				t.Fatalf("expected the migration program to leave the missing %s out, got:\n%s", want, program)
			}
			continue
		}
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
