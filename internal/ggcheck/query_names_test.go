package ggcheck_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
	"github.com/hydroan/gst/internal/modelinspect"
)

// TestQueryNamesRefusesANameDeclaredTwice pins the check: a model declaring
// one query name on two fields is reported under its file with the fix,
// before gg gen wrote any registration, and the same model naming each field
// its own is clean.
func TestQueryNamesRefusesANameDeclaredTwice(t *testing.T) {
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
	writeCheckFile(t, filepath.Join(projectDir, "model", "record.go"), queryNamedModel("title"))

	violations := runCheck(ggcheck.QueryNames)
	want := `model/record.go: model Record: query name "title" is declared by Name and Title; give each field a query name of its own, outside the "_" namespace and without "[", "]" or "."`
	if len(violations) != 1 || violations[0] != want {
		t.Fatalf("violations = %#v, want the name declared twice reported once under its file", violations)
	}

	writeCheckFile(t, filepath.Join(projectDir, "model", "record.go"), queryNamedModel("name"))
	if violations := runCheck(ggcheck.QueryNames); len(violations) != 0 {
		t.Fatalf("a name of its own for each field should have nothing to report, got %#v", violations)
	}
}

// queryNamedModel is a table-backed model whose Name field takes the query
// name nameQuery, beside a Title field taking title.
func queryNamedModel(nameQuery string) string {
	return `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Record is a record.
type Record struct {
	Name  string ` + "`json:\"name\" query:\"" + nameQuery + "\"`" + `
	Title string ` + "`json:\"title\" query:\"title\"`" + `

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
