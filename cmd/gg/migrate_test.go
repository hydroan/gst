package main

import (
	"bytes"
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

// The migration program reads the tables modules register as well as the
// project's own. A module registers its models only once the framework
// releases module registration, and registering mounts its routes on the
// router, so the program has to bring up everything registration runs
// through before it waits for it: dbmigrate.Prepare. Run against a project
// that registers a module, the schema dump lists the module's table. No build
// of this repository compiles the program's source, so this is also what pins
// that it still compiles against the framework.
func TestMigrateSchemaProgramReadsTheTablesModulesRegister(t *testing.T) {
	projectDir := newGenProject(t)
	for _, dir := range constants.ProjectImportDirs {
		content := "package " + dir + "\n"
		if dir == constants.SubDirModule {
			content = "package module\n\nimport \"github.com/hydroan/gst/module/helloworld\"\n\nfunc init() {\n\thelloworld.Register()\n}\n"
		}
		writeCheckFile(t, filepath.Join(projectDir, dir, dir+".go"), content)
	}

	var out bytes.Buffer
	program := projectProgram{Content: buildMigrateSchemaProgram("tmpapp", ""), Stdout: &out}
	if err := program.Run(); err != nil {
		t.Fatalf("expected the migration schema program to run, got %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "CREATE TABLE `helloworld2`") {
		t.Fatalf("expected the schema dump to create the table the module registers, got:\n%s", out.String())
	}
}
