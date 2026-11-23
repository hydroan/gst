package ggcheck_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
)

// TestGormDatatypesBanFlagsTheNamesTheFrameworkHasItsOwnFor pins that every
// type and function of gorm.io/datatypes the framework has its own for is
// reported where project code names it, under its file and line with the
// name to write instead: a model's column, the fields of a request type,
// which nothing else refuses, and a value made in a service, under whatever
// name the file imports the library by. A file importing the library under
// a dot is reported itself.
func TestGormDatatypesBanFlagsTheNamesTheFrameworkHasItsOwnFor(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record.go"), `package record

import (
	"github.com/hydroan/gst/model"
	gormdatatypes "gorm.io/datatypes"
)

type Record struct {
	Day gormdatatypes.Date `+"`json:\"day\"`"+`

	model.Base
}

func (Record) TableName() string { return "records" }
`)
	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "create.go"), `package record

import "gorm.io/datatypes"

type RecordOptions struct {
	Level int `+"`json:\"level\"`"+`
}

type RecordCreateReq struct {
	Opens   datatypes.Time                    `+"`json:\"opens\"`"+`
	Doc     *datatypes.JSON                   `+"`json:\"doc\"`"+`
	Options datatypes.JSONType[RecordOptions] `+"`json:\"options\"`"+`
	Meta    datatypes.JSONMap                 `+"`json:\"meta\"`"+`
	Tags    datatypes.JSONSlice[string]       `+"`json:\"tags\"`"+`
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "create.go"), `package record

import lib "gorm.io/datatypes"

func values() (any, any, any) {
	return lib.NewTime(17, 30, 0, 0), lib.NewJSONType(1), lib.NewJSONSlice([]string{"a"})
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "dao", "record.go"), `package dao

import . "gorm.io/datatypes"

var day Date
`)

	violations := runCheck(ggcheck.GormDatatypesBan)

	const own = " of github.com/hydroan/gst/datatypes"
	modelFile, requestFile, serviceFile := filepath.Join("model", "record", "record.go"), filepath.Join("model", "record", "create.go"), filepath.Join("service", "record", "create.go")
	want := []string{
		filepath.Join("dao", "record.go") + ":3: dot-imports \"gorm.io/datatypes\"; the Gorm datatypes ban cannot tell its names apart, import it under a name",
		requestFile + ":10: gorm.io/datatypes.Time is one the framework has its own for; write datatypes.TimeOfDay" + own,
		requestFile + ":11: gorm.io/datatypes.JSON is one the framework has its own for; write datatypes.JSON[json.RawMessage]" + own,
		requestFile + ":12: gorm.io/datatypes.JSONType is one the framework has its own for; write datatypes.JSON" + own,
		requestFile + ":13: gorm.io/datatypes.JSONMap is one the framework has its own for; write datatypes.JSONMap" + own,
		requestFile + ":14: gorm.io/datatypes.JSONSlice is one the framework has its own for; write datatypes.JSONSlice" + own,
		modelFile + ":9: gorm.io/datatypes.Date is one the framework has its own for; write datatypes.Date" + own,
		serviceFile + ":6: gorm.io/datatypes.NewTime is one the framework has its own for; write datatypes.NewTimeOfDay" + own,
		serviceFile + ":6: gorm.io/datatypes.NewJSONType is one the framework has its own for; write datatypes.NewJSON" + own,
		serviceFile + ":6: gorm.io/datatypes.NewJSONSlice is one the framework has its own for; write datatypes.NewJSONSlice" + own,
	}
	if !slices.Equal(violations, want) {
		t.Fatalf("violations = %#v, want %#v", violations, want)
	}
}

// TestGormDatatypesBanLeavesTheRestOfTheLibraryAndTheFrameworksOwn pins what
// the ban leaves alone: the names of the library the framework has nothing
// for, a JSON query and a UUID among them, the framework's own types under
// the package name the two share, another package imported under a dot, a
// name of the project's own that reads like one of the library's, test
// files, and a file gg generated from a model that declared the library's
// type, which the gg gen run the check precedes is there to rewrite.
func TestGormDatatypesBanLeavesTheRestOfTheLibraryAndTheFrameworksOwn(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record.go"), `package record

import (
	"github.com/hydroan/gst/datatypes"
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	gormdatatypes "gorm.io/datatypes"
)

type Record struct {
	Day  datatypes.Date              `+"`json:\"day\"`"+`
	Tags datatypes.JSONSlice[string] `+"`json:\"tags\"`"+`
	Ref  gormdatatypes.UUID          `+"`json:\"ref\"`"+`

	model.Base
}

func (Record) TableName() string { return "records" }

func (Record) Design() {
	Migrate()
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "list.go"), `package record

import (
	"time"

	"gorm.io/datatypes"
)

type clock struct{ Time time.Time }

func query(c clock) (any, time.Time) {
	return datatypes.JSONQuery("meta").HasKey("level"), c.Time
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "list_test.go"), `package record

import "gorm.io/datatypes"

var fixture datatypes.Date
`)
	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record.gen.go"), `package record

import (
	"github.com/hydroan/gst"
	"gorm.io/datatypes"
)

var RecordCols = struct {
	Meta gst.Column[datatypes.JSONMap]
}{
	Meta: gst.NewColumn[*Record, datatypes.JSONMap]("meta"),
}
`)

	violations := runCheck(ggcheck.GormDatatypesBan)

	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %#v", violations)
	}
}
