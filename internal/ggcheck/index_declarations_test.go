package ggcheck_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
	"github.com/hydroan/gst/internal/modelinspect"
)

// TestIndexDeclarationsRefusesAColumnMySQLCannotIndex pins the check on a
// project configured for MySQL: a model indexing a string field without a
// size is reported under its file with the fix, before gg gen wrote any
// registration; the same field with a size is clean; and a project configured
// for another database is not held to the MySQL rule.
func TestIndexDeclarationsRefusesAColumnMySQLCannotIndex(t *testing.T) {
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
	writeCheckFile(t, filepath.Join(projectDir, "config.ini"), "[database]\ntype = mysql\n")
	writeCheckFile(t, filepath.Join(projectDir, "model", "record.go"), indexedRecordModel(""))

	violations := runCheck(ggcheck.IndexDeclarations)
	want := `model/record.go: model Record: index field Owner is a longtext column, which MySQL cannot index without a key length; give it a size (gorm:"size:191") or a bounded type`
	if len(violations) != 1 || !strings.Contains(violations[0], want) {
		t.Fatalf("violations = %#v, want the unsized indexed field reported once under its file", violations)
	}

	writeCheckFile(t, filepath.Join(projectDir, "model", "record.go"), indexedRecordModel(` gorm:"size:36"`))
	if violations := runCheck(ggcheck.IndexDeclarations); len(violations) != 0 {
		t.Fatalf("a sized field should have nothing to report, got %#v", violations)
	}

	writeCheckFile(t, filepath.Join(projectDir, "model", "record.go"), indexedRecordModel(""))
	writeCheckFile(t, filepath.Join(projectDir, "config.ini"), "[database]\ntype = sqlite\n")
	if violations := runCheck(ggcheck.IndexDeclarations); len(violations) != 0 {
		t.Fatalf("a project on another database is not held to the MySQL rule, got %#v", violations)
	}
}

// indexedRecordModel is a table-backed model indexing its Owner field, the
// field's struct tag extended by ownerTag after its json tag; "" leaves the
// string without a size.
func indexedRecordModel(ownerTag string) string {
	return `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Record is a record.
type Record struct {
	Owner string ` + "`json:\"owner\"" + ownerTag + "`" + `

	model.Base
}

func (Record) TableName() string { return "records" }

func (Record) Indexes() []model.Index { return []model.Index{{Fields: []string{"Owner"}}} }

func (Record) Design() {
	dsl.Migrate()
	dsl.Endpoint("records")
	dsl.Create(func() {})
}
`
}
