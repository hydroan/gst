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

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
)

// The migration program's model set is whatever the linked packages
// registered, so it has to link the project packages a generated main.go
// links and the framework entry point that links the rest, and it has to
// initialize the router and modules through that entry point instead of
// guessing when module registration is done. A
// scaffold package the project lacks — restored by gg gen, never by migrate
// — is left out rather than failing the build, which is what a project that
// has not run gen yet looks like.
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
	if !newMigrateSampleProject(t) {
		return
	}

	out := runMigrateSchemaProgramForTest(t, "", nil)

	if !strings.Contains(out, "CREATE TABLE `samples`") {
		t.Fatalf("expected the schema dump to create the table the module registers, got:\n%s", out)
	}
}

// With a source, the program dumps the registered models declared in the
// files gg listed under it: here the module package, which declares the
// model it registers.
func TestMigrateSchemaProgramReadsTheModelsItsSourceDeclares(t *testing.T) {
	if !newMigrateSampleProject(t) {
		return
	}
	files, err := migrateSourceFiles(ggconst.DirModule, gghelper.NewProjectIgnore())
	if err != nil {
		t.Fatal(err)
	}

	out := runMigrateSchemaProgramForTest(t, ggconst.DirModule, files)

	if !strings.Contains(out, "CREATE TABLE `samples`") {
		t.Fatalf("expected the schema dump to create the table of the model the source declares, got:\n%s", out)
	}
}

// On an in-memory sqlite database, the default configuration, the migration
// program has nothing to migrate: the application creates the tables from the
// models every time it starts, and the program, running in a process of its
// own, cannot reach that database. It says so, and opens no database file:
// the one file it writes is the schema snapshot.
func TestMigrateProgramHasNothingToMigrateOnAnInMemorySqliteDatabase(t *testing.T) {
	if !newMigrateSampleProject(t) {
		return
	}
	for _, key := range []string{config.SQLITE_IS_MEMORY, config.SQLITE_PATH} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	before := projectTree(t, ".")

	out := runMigrateProgramForTest(t, true, false)

	for _, want := range []string{"→ Target: in-memory database", "→ Nothing to migrate:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected the migration output to contain %q, got:\n%s", want, out)
		}
	}
	requireOnlySchemaSnapshotWritten(t, before)
}

// Planning against a sqlite database file that does not exist yet creates
// nothing: the plan starts from the empty database the file would be, and the
// program says applying it creates the file.
func TestMigrateProgramPlansAMissingSqliteFileWithoutCreatingIt(t *testing.T) {
	if !newMigrateSampleProject(t) {
		return
	}
	t.Setenv(config.SQLITE_IS_MEMORY, "false")
	t.Setenv(config.SQLITE_PATH, "./data.db")
	before := projectTree(t, ".")

	out := runMigrateProgramForTest(t, true, false)

	for _, want := range []string{"CREATE TABLE `samples`", missingSqliteFileNotice} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected the migration output to contain %q, got:\n%s", want, out)
		}
	}
	requireOnlySchemaSnapshotWritten(t, before)
}

// On a sqlite database file the migration program migrates the file
// sqlite.path names, the database the application opens: applied, the plan
// creates the file and the tables there, and planned again, it finds nothing
// to change.
func TestMigrateProgramMigratesTheSqliteFileThePathNames(t *testing.T) {
	if !newMigrateSampleProject(t) {
		return
	}
	t.Setenv(config.SQLITE_IS_MEMORY, "false")
	t.Setenv(config.SQLITE_PATH, "./data.db")

	out := runMigrateProgramForTest(t, false, true)

	for _, want := range []string{"→ Target: ./data.db", missingSqliteFileNotice, "✔ Migration executed successfully."} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected the applied migration's output to contain %q, got:\n%s", want, out)
		}
	}

	out = runMigrateProgramForTest(t, true, false)

	for _, want := range []string{"→ Target: ./data.db", "→ No changes detected."} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected the planned migration's output to contain %q, got:\n%s", want, out)
		}
	}
}

// TestMigrateSourceFiles pins the files gg migrate schema reads model types
// from. Below a directory, every Go file that is not a test, a model package
// named generated among them, and none the go command leaves out: files and
// directories whose names begin with "." or "_", vendor and testdata
// directories, nested modules, and a directory the project's go.mod ignores.
// The project's Git ignore rules leave nothing out, the user having named the
// source. A Go file named on its own is read alone. A link to a directory is
// not followed, as no walk over the project's code follows one: a linked
// directory below the source adds nothing, and a source that is itself such a
// link holds no Go file. A link to a Go file is read as that file, below the
// source or named on its own, the way gg gen reads it.
func TestMigrateSourceFiles(t *testing.T) {
	outside := t.TempDir()
	writeProjectFile(t, filepath.Join(outside, "linked.go"), "package linked\n")
	t.Chdir(t.TempDir())
	for path, content := range map[string]string{
		"go.mod":                    "module example.com/app\n\ngo 1.27\n\nignore ./model/assets\n",
		".gitignore":                "model/generated/\n",
		"model/record.go":           "package model\n",
		"model/record_test.go":      "package model\n",
		"model/_draft.go":           "package model\n",
		"model/generated/sample.go": "package generated\n",
		"model/.cache/cached.go":    "package cached\n",
		"model/_old/old.go":         "package old\n",
		"model/assets/asset.go":     "package assets\n",
		"model/vendor/lib/lib.go":   "package lib\n",
		"model/testdata/fixture.go": "package fixture\n",
		"model/nested/go.mod":       "module example.com/nested\n",
		"model/nested/nested.go":    "package nested\n",
	} {
		writeProjectFile(t, filepath.FromSlash(path), content)
	}
	for link, target := range map[string]string{
		filepath.Join("model", "linked"):   outside,
		filepath.Join("model", "alias.go"): filepath.Join(outside, "linked.go"),
		"model-link":                       ggconst.DirModel,
		"record-link.go":                   filepath.Join("model", "record.go"),
	} {
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
	}

	ignore := gghelper.NewProjectIgnore()
	files, err := migrateSourceFiles(ggconst.DirModel, ignore)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join("model", "alias.go"), filepath.Join("model", "generated", "sample.go"), filepath.Join("model", "record.go")}
	if !slices.Equal(files, want) {
		t.Fatalf("migrateSourceFiles(%q) = %q, want %q", ggconst.DirModel, files, want)
	}

	single := filepath.Join("model", "record.go")
	files, err = migrateSourceFiles(single, ignore)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(files, []string{single}) {
		t.Fatalf("migrateSourceFiles(%q) = %q, want the file alone", single, files)
	}

	files, err = migrateSourceFiles("model-link", ignore)
	if err == nil || !strings.Contains(err.Error(), "no Go model files found under model-link") {
		t.Fatalf("migrateSourceFiles(%q) = %q, %v, want no Go model files found", "model-link", files, err)
	}

	files, err = migrateSourceFiles("record-link.go", ignore)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(files, []string{"record-link.go"}) {
		t.Fatalf("migrateSourceFiles(%q) = %q, want the linked file alone", "record-link.go", files)
	}
}

// newMigrateSampleProject creates a project for the migration program tests
// whose module package registers migrateSampleModule's model, and pins the
// dialect the dump is rendered in: the program reads the environment, so a
// DATABASE_TYPE set where the tests run cannot change what the assertions
// read. It reports whether the test goes on, the way newGenProject does.
func newMigrateSampleProject(t *testing.T) bool {
	t.Helper()

	projectDir, ok := newGenProject(t)
	if !ok {
		return false
	}
	t.Setenv("DATABASE_TYPE", "sqlite")
	for _, dir := range ggconst.ProjectImportDirs {
		content := "package " + dir + "\n"
		if dir == ggconst.DirModule {
			content = migrateSampleModule
		}
		writeProjectFile(t, filepath.Join(projectDir, dir, dir+".go"), content)
	}
	return true
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

// runMigrateProgramForTest builds the migration program in the mode the
// --dry-run and --yes flags select, runs it in the working directory and
// returns what it printed.
func runMigrateProgramForTest(t *testing.T, dryRun, yes bool) string {
	t.Helper()

	oldDryRun, oldYes := migrateDryRun, migrateYes
	migrateDryRun, migrateYes = dryRun, yes
	defer func() { migrateDryRun, migrateYes = oldDryRun, oldYes }()

	var out bytes.Buffer
	program := gghelper.ProjectProgram{Content: buildMigrateProgramForMode("tmpapp", false, "", nil), Stdout: &out}
	if err := program.Run(); err != nil {
		t.Fatalf("expected the migration program to run, got %v\n%s", err, out.String())
	}
	return out.String()
}

// missingSqliteFileNotice is what the migration program prints below a plan
// for ./data.db while the file does not exist yet.
const missingSqliteFileNotice = "→ ./data.db does not exist yet: the plan starts from an empty database, and applying it creates the file."

// requireOnlySchemaSnapshotWritten fails the test unless the one file the
// working directory holds beyond the files of before, a projectTree taken
// before the migration program ran, is the schema snapshot.
func requireOnlySchemaSnapshotWritten(t *testing.T, before map[string]string) {
	t.Helper()

	var written []string
	for path := range projectTree(t, ".") {
		if _, existed := before[path]; !existed {
			written = append(written, path)
		}
	}
	slices.Sort(written)
	want := []string{filepath.Join("generated", "migrate", "sqlite", "schema.sql")}
	if !slices.Equal(written, want) {
		t.Fatalf("expected the migration program to write %q alone, wrote %q", want, written)
	}
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
	gstmodule.Use(gstmodule.NewWrapper[*Sample, *Sample, *Sample]("samples", "id", false, &SampleService{}), gstmodule.CRUD(consts.List))
}
`
