package main

import (
	"bytes"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
)

// The migration program's model set is whatever the linked packages
// registered, so it has to link the project packages a generated main.go
// links and the framework entry point that links the rest, and it has to
// initialize the router and modules through that entry point instead of
// guessing when module registration is done. A
// scaffold package the project lacks — restored by gg gen, never by migrate
// — is left out rather than failing the build, which is what an upgraded
// project that has not run gen yet looks like.
func TestMigrateProgramLinksWhatMainLinks(t *testing.T) {
	projectDir := t.TempDir()
	for _, dir := range ggconst.ProjectImportDirs {
		if dir == ggconst.DirLock {
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

	program := buildMigrateProgramForMode("sample", false, "", nil)

	for _, dir := range ggconst.ProjectImportDirs {
		want := fmt.Sprintf("_ %q", "sample/"+dir)
		if dir == ggconst.DirLock {
			if strings.Contains(program, want) {
				t.Fatalf("expected the migration program to leave the missing %s out, got:\n%s", want, program)
			}
			continue
		}
		if !strings.Contains(program, want) {
			t.Fatalf("expected the migration program to import %s, got:\n%s", want, program)
		}
	}
	for _, want := range []string{`"github.com/hydroan/gst/bootstrap"`, "bootstrap.InitRouterAndModules()"} {
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
// router, so the program has to bring up the router and the modules and wait
// for their registration: bootstrap.InitRouterAndModules. Run against a
// project that registers a module, the schema dump lists the module's table.
// No build of this repository compiles the program's source, so this is also
// what pins that it still compiles against the framework.
func TestMigrateSchemaProgramReadsTheTablesModulesRegister(t *testing.T) {
	newMigrateSampleProject(t)

	out := runMigrateSchemaProgramForTest(t, "", nil)

	if !strings.Contains(out, "CREATE TABLE `samples`") {
		t.Fatalf("expected the schema dump to create the table the module registers, got:\n%s", out)
	}
}

// With a source, the program dumps the registered models declared in the
// files gg listed under it: here the module package, which declares the
// model it registers.
func TestMigrateSchemaProgramReadsTheModelsItsSourceDeclares(t *testing.T) {
	newMigrateSampleProject(t)
	files, err := migrateSourceFiles(ggconst.DirModule)
	if err != nil {
		t.Fatal(err)
	}

	out := runMigrateSchemaProgramForTest(t, ggconst.DirModule, files)

	if !strings.Contains(out, "CREATE TABLE `samples`") {
		t.Fatalf("expected the schema dump to create the table of the model the source declares, got:\n%s", out)
	}
}

// TestMigrateSourceFiles pins the files gg migrate schema reads model types
// from. Below a directory, every Go file that is not a test, a model package
// named generated among them, and none a walk over the project's code leaves
// out: hidden, vendor and testdata directories and nested modules. A Go file
// named on its own is read alone.
func TestMigrateSourceFiles(t *testing.T) {
	t.Chdir(t.TempDir())
	for path, content := range map[string]string{
		"model/record.go":           "package model\n",
		"model/record_test.go":      "package model\n",
		"model/generated/sample.go": "package generated\n",
		"model/.cache/cached.go":    "package cached\n",
		"model/vendor/lib/lib.go":   "package lib\n",
		"model/testdata/fixture.go": "package fixture\n",
		"model/nested/go.mod":       "module example.com/nested\n",
		"model/nested/nested.go":    "package nested\n",
	} {
		writeProjectFile(t, filepath.FromSlash(path), content)
	}

	files, err := migrateSourceFiles(ggconst.DirModel)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join("model", "generated", "sample.go"), filepath.Join("model", "record.go")}
	if !slices.Equal(files, want) {
		t.Fatalf("migrateSourceFiles(%q) = %q, want %q", ggconst.DirModel, files, want)
	}

	single := filepath.Join("model", "record.go")
	files, err = migrateSourceFiles(single)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(files, []string{single}) {
		t.Fatalf("migrateSourceFiles(%q) = %q, want the file alone", single, files)
	}
}

// newMigrateSampleProject creates a project for the migration program tests
// whose module package registers migrateSampleModule's model, and pins the
// dialect the dump is rendered in: the program reads the environment, so a
// DATABASE_TYPE set where the tests run cannot change what the assertions
// read.
func newMigrateSampleProject(t *testing.T) {
	t.Helper()

	projectDir := newGenProject(t)
	t.Setenv("DATABASE_TYPE", "sqlite")
	for _, dir := range ggconst.ProjectImportDirs {
		content := "package " + dir + "\n"
		if dir == ggconst.DirModule {
			content = migrateSampleModule
		}
		writeProjectFile(t, filepath.Join(projectDir, dir, dir+".go"), content)
	}
}

// runMigrateSchemaProgramForTest builds the schema program for source and the
// files listed under it, runs it in the working directory and returns what it
// printed.
func runMigrateSchemaProgramForTest(t *testing.T, source string, files []string) string {
	t.Helper()

	var out bytes.Buffer
	program := gghelper.ProjectProgram{Content: buildMigrateSchemaProgram("tmpapp", source, files), Stdout: &out}
	if err := program.Run(); err != nil {
		t.Fatalf("expected the migration schema program to run, got %v\n%s", err, out.String())
	}
	return out.String()
}

// migrateSampleModule is the module package of a project that registers one
// module of its own, whose model has a table.
const migrateSampleModule = `package module

import (
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/model"
	gstmodule "github.com/hydroan/gst/module"
	"github.com/hydroan/gst/service"
)

type Sample struct {
	Name string ` + "`json:\"name\"`" + `

	model.Base
}

func (Sample) TableName() string { return "samples" }

type SampleService struct {
	service.Base[*Sample, *Sample, *Sample]
}

func init() {
	gstmodule.Use(gstmodule.NewWrapper[*Sample, *Sample, *Sample]("samples", "id", false, &SampleService{}), gstmodule.CRUD(consts.PHASE_LIST))
}
`
