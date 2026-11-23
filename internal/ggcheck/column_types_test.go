package ggcheck_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
	"github.com/hydroan/gst/internal/modelinspect"
)

// TestColumnTypesRefusesAColumnOfATypeTheFrameworkHasItsOwnFor pins the
// check: a model declaring a day and an array by the types of
// gorm.io/datatypes and a day by a time.Time under a type tag is reported
// under its file with the declarations to write, before gg gen wrote any
// registration, and the same columns declared by the framework's types are
// clean.
func TestColumnTypesRefusesAColumnOfATypeTheFrameworkHasItsOwnFor(t *testing.T) {
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
	writeCheckFile(t, filepath.Join(projectDir, "model", "record.go"), libraryTypedModel)

	violations := runCheck(ggcheck.ColumnTypes)
	want := `model/record.go: model Record: field Day is declared gorm.io/datatypes.Date, declare it datatypes.Date; field Due is a date column declared *time.Time, declare it *datatypes.Date; field Tags is declared gorm.io/datatypes.JSONSlice, declare it datatypes.JSONSlice; datatypes is github.com/hydroan/gst/datatypes`
	if len(violations) != 1 || violations[0] != want {
		t.Fatalf("violations = %#v, want the columns reported once under their file", violations)
	}

	writeCheckFile(t, filepath.Join(projectDir, "model", "record.go"), frameworkTypedModel)
	if violations := runCheck(ggcheck.ColumnTypes); len(violations) != 0 {
		t.Fatalf("columns declared by the framework's types should have nothing to report, got %#v", violations)
	}
}

// libraryTypedModel is a table-backed model declaring a day and an array by
// the types of gorm.io/datatypes and an optional day by a time.Time a type
// tag declares a date.
const libraryTypedModel = `package model

import (
	"time"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"gorm.io/datatypes"
)

// Record is a record of one day.
type Record struct {
	Day  datatypes.Date              ` + "`json:\"day\"`" + `
	Due  *time.Time                  ` + "`json:\"due\" gorm:\"type:date\"`" + `
	Tags datatypes.JSONSlice[string] ` + "`json:\"tags\"`" + `

	model.Base
}

func (Record) TableName() string { return "records" }

func (Record) Design() {
	dsl.Migrate()
	dsl.Endpoint("records")
	dsl.Create(func() {})
}
`

// frameworkTypedModel is the same model declaring its columns by the
// framework's own types.
const frameworkTypedModel = `package model

import (
	"github.com/hydroan/gst/datatypes"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Record is a record of one day.
type Record struct {
	Day  datatypes.Date              ` + "`json:\"day\"`" + `
	Due  *datatypes.Date             ` + "`json:\"due\"`" + `
	Tags datatypes.JSONSlice[string] ` + "`json:\"tags\"`" + `

	model.Base
}

func (Record) TableName() string { return "records" }

func (Record) Design() {
	dsl.Migrate()
	dsl.Endpoint("records")
	dsl.Create(func() {})
}
`
