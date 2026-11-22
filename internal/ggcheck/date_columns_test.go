package ggcheck_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
	"github.com/hydroan/gst/internal/modelinspect"
)

// TestDateColumnsRefusesADateColumnOfAnotherType pins the check: a model
// declaring its date columns by the date type of gorm.io/datatypes and by a
// time.Time under a type tag is reported under its file with the fix, before
// gg gen wrote any registration, and the same columns declared model.Date
// are clean.
func TestDateColumnsRefusesADateColumnOfAnotherType(t *testing.T) {
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
	writeCheckFile(t, filepath.Join(projectDir, "model", "record.go"), libraryDatedModel)

	violations := runCheck(ggcheck.DateColumns)
	want := `model/record.go: model Record: field Day stores a date as datatypes.Date; field Due stores a date as *time.Time; a date column is declared model.Date, or *model.Date when it is optional`
	if len(violations) != 1 || violations[0] != want {
		t.Fatalf("violations = %#v, want the date columns reported once under their file", violations)
	}

	writeCheckFile(t, filepath.Join(projectDir, "model", "record.go"), datedModel)
	if violations := runCheck(ggcheck.DateColumns); len(violations) != 0 {
		t.Fatalf("date columns declared model.Date should have nothing to report, got %#v", violations)
	}
}

// libraryDatedModel is a table-backed model declaring a day by the date type
// of gorm.io/datatypes and an optional one by a time.Time a type tag declares
// a date.
const libraryDatedModel = `package model

import (
	"time"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"gorm.io/datatypes"
)

// Record is a record of one day.
type Record struct {
	Day datatypes.Date ` + "`json:\"day\"`" + `
	Due *time.Time     ` + "`json:\"due\" gorm:\"type:date\"`" + `

	model.Base
}

func (Record) TableName() string { return "records" }

func (Record) Design() {
	dsl.Migrate()
	dsl.Endpoint("records")
	dsl.Create(func() {})
}
`

// datedModel is the same model declaring its days as the framework's own.
const datedModel = `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Record is a record of one day.
type Record struct {
	Day model.Date  ` + "`json:\"day\"`" + `
	Due *model.Date ` + "`json:\"due\"`" + `

	model.Base
}

func (Record) TableName() string { return "records" }

func (Record) Design() {
	dsl.Migrate()
	dsl.Endpoint("records")
	dsl.Create(func() {})
}
`
