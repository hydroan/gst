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

	program := buildMigrateProgramForMode("sample", false, "")

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
	projectDir := newGenProject(t)
	// The dump is rendered in the dialect the configuration names, and the
	// program reads the environment: pin the default, so a DATABASE_TYPE set
	// where the tests run cannot change what the assertion reads.
	t.Setenv("DATABASE_TYPE", "sqlite")
	for _, dir := range ggconst.ProjectImportDirs {
		content := "package " + dir + "\n"
		if dir == ggconst.DirModule {
			content = migrateSampleModule
		}
		writeProjectFile(t, filepath.Join(projectDir, dir, dir+".go"), content)
	}

	var out bytes.Buffer
	program := gghelper.ProjectProgram{Content: buildMigrateSchemaProgram("tmpapp", ""), Stdout: &out}
	if err := program.Run(); err != nil {
		t.Fatalf("expected the migration schema program to run, got %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "CREATE TABLE `samples`") {
		t.Fatalf("expected the schema dump to create the table the module registers, got:\n%s", out.String())
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
	gstmodule.Use(gstmodule.NewWrapper[*Sample, *Sample, *Sample]("samples", "id", false, &SampleService{}), gstmodule.CRUD(consts.PHASE_LIST))
}
`
