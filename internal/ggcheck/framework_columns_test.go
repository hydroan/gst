package ggcheck_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
	"github.com/hydroan/gst/internal/ggmodule"
	"github.com/hydroan/gst/internal/modelinspect"
)

// TestFrameworkColumnsRefusesARenamedFrameworkColumn pins the check: a model
// re-declaring the soft-delete field under a column of its own is reported
// under its file with the fix, before gg gen wrote any registration, and the
// same field re-declared under the column the framework fixes is clean.
func TestFrameworkColumnsRefusesARenamedFrameworkColumn(t *testing.T) {
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
	writeCheckFile(t, filepath.Join(projectDir, "model", "record.go"), redeclaredDeletedAtModel(` gorm:"column:removed_at"`))

	violations := runCheck(ggcheck.FrameworkColumns)
	want := `model/record.go: model Record: framework field DeletedAt maps to column "removed_at" instead of "deleted_at"; the framework fixes the names of its columns, so drop the column tag or embedded prefix renaming them`
	if len(violations) != 1 || violations[0] != want {
		t.Fatalf("violations = %#v, want the renamed column reported once under its file", violations)
	}

	writeCheckFile(t, filepath.Join(projectDir, "model", "record.go"), redeclaredDeletedAtModel(""))
	if violations := runCheck(ggcheck.FrameworkColumns); len(violations) != 0 {
		t.Fatalf("the column the framework fixes should have nothing to report, got %#v", violations)
	}
}

// TestFrameworkColumnsSkipsTheModelsOfCopiedModules pins that a model under a
// subtree a copyable framework module owns is left to the framework
// repository, which checks copied module code itself, while the same model
// anywhere else is reported. The module is the first one the framework
// offers, so the test names none of them.
func TestFrameworkColumnsSkipsTheModelsOfCopiedModules(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
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
	modules, err := ggmodule.CopyableModuleNames()
	if err != nil {
		t.Fatal(err)
	}
	if len(modules) == 0 {
		t.Skip("the framework offers no copyable module")
	}
	writeCheckFile(t, filepath.Join(projectDir, "model", "model.go"), "package model\n")
	for _, pkg := range []string{modules[0], "sample"} {
		source := strings.Replace(redeclaredDeletedAtModel(` gorm:"column:removed_at"`), "package model\n", "package "+pkg+"\n", 1)
		writeCheckFile(t, filepath.Join(projectDir, "model", pkg, "record.go"), source)
	}

	violations := runCheck(ggcheck.FrameworkColumns)
	want := `model/sample/record.go: model Record: framework field DeletedAt maps to column "removed_at" instead of "deleted_at"; the framework fixes the names of its columns, so drop the column tag or embedded prefix renaming them`
	if len(violations) != 1 || violations[0] != want {
		t.Fatalf("violations = %#v, want only the model outside the subtree of module %s reported", violations, modules[0])
	}
}

// TestFrameworkColumnsLeavesARefusedScanToTheDSLDesignRules pins that model
// files the scan refuses are reported by the DSL design rules alone: the
// check, unable to read the models, adds nothing of its own.
func TestFrameworkColumnsLeavesARefusedScanToTheDSLDesignRules(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)
	writeCheckFile(t, filepath.Join(projectDir, "model", "record.go"), "package model\n\ntype Record struct {\n")

	if violations := runCheck(ggcheck.FrameworkColumns); len(violations) != 0 {
		t.Fatalf("a refused scan is the DSL design rules' to report, got %#v", violations)
	}
}

// redeclaredDeletedAtModel is a table-backed model re-declaring the
// soft-delete field, the field's struct tag extended by deletedAtTag after its
// json tag; "" leaves the column the framework fixes.
func redeclaredDeletedAtModel(deletedAtTag string) string {
	return `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"gorm.io/gorm"
)

// Record is a record.
type Record struct {
	DeletedAt gorm.DeletedAt ` + "`json:\"-\"" + deletedAtTag + "`" + `

	model.Base
}

func (Record) TableName() string { return "records" }

func (Record) Design() {
	dsl.Migrate()
	dsl.Endpoint("records")
	dsl.Create(func() {})
}
`
}
