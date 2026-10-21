package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/hydroan/gst/internal/modelinspect"
	"github.com/stretchr/testify/require"
)

// TestGenRunGeneratesColumnsReadByHandwrittenModelCode runs gg gen against a
// project whose handwritten model code reads generated column references: a
// hook, package-level vars read by an init function and by other functions,
// the time buckets of a date column and of an optional instant, and a hook
// of another model package. The inspection build compiles that code before
// the run writes the references, first with no column file at all and then
// with the previous generation stubbed out; the references the run writes
// must then satisfy the same code in the project's own build, which is what
// pins the date column to the time reference: a plain reference has no
// bucket to read.
func TestGenRunGeneratesColumnsReadByHandwrittenModelCode(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	writeProjectFile(t, filepath.Join(projectDir, "model", "sample", "record.go"), `package sample

import (
	"context"
	"database/sql"
	"time"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"gorm.io/datatypes"
)

// RecordKind is a type of the model's own package, which a column of a
// composite type names.
type RecordKind string

type Record struct {
	Status   string              `+"`json:\"status\"`"+`
	Score    int64               `+"`json:\"score\"`"+`
	Day      datatypes.Date      `+"`json:\"day\"`"+`
	ClosedAt *time.Time          `+"`json:\"closed_at\"`"+`
	Settled  sql.NullTime        `+"`json:\"settled\"`"+`
	Tally    sql.NullInt64       `+"`json:\"tally\"`"+`
	Clock    datatypes.Time      `+"`json:\"clock\"`"+`
	Times    []time.Time         `+"`json:\"times\" gorm:\"serializer:json\"`"+`
	Kinds    []RecordKind        `+"`json:\"kinds\" gorm:\"serializer:json\"`"+`
	ByKind   map[RecordKind]int64 `+"`json:\"by_kind\" gorm:\"serializer:json\"`"+`

	model.Base
}

func (Record) TableName() string { return "records" }

func (Record) Design() {
	dsl.Migrate()
}

func (r *Record) CreateBefore(ctx context.Context) error {
	return database.Database[*Record](ctx).UpdateByID(r.ID, RecordCols.Status.Set("active"))
}
`)
	writeProjectFile(t, filepath.Join(projectDir, "model", "sample", "status.go"), `package sample

import (
	"math/rand/v2"
	"strings"

	"github.com/hydroan/gst"
)

var (
	defaultStatus = "active"
	statusColumn  = RecordCols.Status
)

var sortableColumns = []gst.AnyColumnRef{statusColumn, RecordCols.Score}

// The date column, the optional instant and the times sql.NullTime and the
// soft-delete timestamp hold are time references: they bucket. The number
// sql.NullInt64 holds is a numeric reference: it sums.
var reportKeys = []gst.Term{
	RecordCols.Day.ByDay(), RecordCols.ClosedAt.ByMonth(),
	RecordCols.Settled.ByDay(), RecordCols.DeletedAt.ByMonth(),
}

var reportTotals = []gst.Term{RecordCols.Tally.Sum()}

func init() {
	if strings.TrimSpace(statusColumn.Name()) == "" {
		panic("status column has no name")
	}
}

func statusFilter() gst.Filter {
	return statusColumn.Eq(defaultStatus + strings.Repeat("!", rand.IntN(2)))
}

func sortColumns() []gst.AnyColumnRef {
	return sortableColumns
}

func reportTermCount() int {
	return len(reportKeys) + len(reportTotals)
}
`)
	writeProjectFile(t, filepath.Join(projectDir, "model", "item", "item.go"), `package item

import (
	"context"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"

	recordmodel "tmpapp/model/sample"
)

type Item struct {
	RecordID string `+"`json:\"record_id\"`"+`

	model.Base
}

func (Item) TableName() string { return "items" }

func (Item) Design() {
	dsl.Migrate()
}

func (i *Item) DeleteBefore(ctx context.Context) error {
	return database.Database[*recordmodel.Record](ctx).UpdateByID(i.RecordID, recordmodel.RecordCols.Status.Set("detached"))
}
`)

	cacheDir, err := modelinspect.CacheDir()
	require.NoError(t, err)
	recordColumnsFile := filepath.Join("model", "sample", "record.gen.go")
	itemColumnsFile := filepath.Join("model", "item", "item.gen.go")

	// First run: no column file exists yet, so nothing declares the
	// references the handwritten code reads.
	require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))
	recordColumns, err := os.ReadFile(recordColumnsFile)
	require.NoError(t, err)
	require.Contains(t, string(recordColumns), "var RecordCols = struct")
	require.Contains(t, string(recordColumns), `gst.NewTimeColumn[*Record]("day")`, "a date column gets the time reference")
	require.Contains(t, string(recordColumns), `gst.NewTimeColumn[*Record]("closed_at")`, "an optional instant gets the time reference")
	// gorm reads a Valuer through the value it holds, and so does the
	// reference gg gen writes for it.
	require.Contains(t, string(recordColumns), `gst.NewTimeColumn[*Record]("settled")`, "the time sql.NullTime holds gets the time reference")
	require.Contains(t, string(recordColumns), `gst.NewTimeColumn[*Record]("deleted_at")`, "so does the soft-delete timestamp")
	require.Contains(t, string(recordColumns), `gst.NewNumericColumn[*Record, sql.NullInt64]("tally")`, "the number sql.NullInt64 holds gets the numeric reference")
	require.Contains(t, string(recordColumns), `gst.NewColumn[*Record, datatypes.Time]("clock")`, "a time of day neither sums nor buckets")
	// A composite type is written from its elements: a type of another
	// package qualified and imported, one of the model's own package bare.
	require.Contains(t, string(recordColumns), `gst.NewColumn[*Record, []time.Time]("times")`)
	require.Contains(t, string(recordColumns), "\t\"time\"\n", "the import of a type an element names")
	require.Contains(t, string(recordColumns), `gst.NewColumn[*Record, []RecordKind]("kinds")`)
	require.Contains(t, string(recordColumns), `gst.NewColumn[*Record, map[RecordKind]int64]("by_kind")`)
	itemColumns, err := os.ReadFile(itemColumnsFile)
	require.NoError(t, err)
	require.Contains(t, string(itemColumns), "var ItemCols = struct")

	// The written references satisfy the handwritten readers.
	requireProjectCompiles(t)

	// Second run: the previous column files are stubbed out of the inspection
	// build, which has to resolve the same columns again. Dropping the cached
	// result makes the inspection actually run.
	require.NoError(t, os.RemoveAll(cacheDir))
	require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))
	regeneratedRecordColumns, err := os.ReadFile(recordColumnsFile)
	require.NoError(t, err)
	require.Equal(t, string(recordColumns), string(regeneratedRecordColumns))
	regeneratedItemColumns, err := os.ReadFile(itemColumnsFile)
	require.NoError(t, err)
	require.Equal(t, string(itemColumns), string(regeneratedItemColumns))
}

// TestGenRunReferencesFrameworkTypesThroughTheRootPackage runs gg gen
// against a model whose column is typed by the framework's root package. The
// generated reference names the type the way the model source does, under the
// import path a business project can reach.
func TestGenRunReferencesFrameworkTypesThroughTheRootPackage(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	writeProjectFile(t, filepath.Join(projectDir, "model", "sample", "rule.go"), `package sample

import (
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Rule struct {
	Permission gst.Permission `+"`json:\"permission\" gorm:\"serializer:json\"`"+`

	model.Base
}

func (Rule) TableName() string { return "rules" }

func (Rule) Design() {
	dsl.Migrate()
}
`)

	require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))
	columnFile, err := os.ReadFile(filepath.Join("model", "sample", "rule.gen.go"))
	require.NoError(t, err)
	require.Contains(t, string(columnFile), `gst.NewColumn[*Rule, gst.Permission]("permission")`)

	// The project only compiles when every import the generated file carries
	// is one it can reach.
	requireProjectCompiles(t)
}

// TestGenRunReportsCodeReachingAColumnInspectionPlaceholder runs gg gen against
// a project whose package initialization calls a method that reads generated
// column references. Methods are never left out by name, so the call stays in
// the inspection build and reaches the placeholder that replaced the method
// body: generation fails, and what the terminal shows has to say why and point
// at the source lines involved.
func TestGenRunReportsCodeReachingAColumnInspectionPlaceholder(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	source := `package sample

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	Status string ` + "`json:\"status\"`" + `

	model.Base
}

func (Record) TableName() string { return "records" }

func (Record) Design() {
	dsl.Migrate()
}

func (r *Record) statusColumnName() string {
	return RecordCols.Status.Name()
}

var defaultStatusColumn = (&Record{}).statusColumnName()
`
	writeProjectFile(t, filepath.Join(projectDir, "model", "sample", "record.go"), source)

	var genErr error
	stderr := captureStderr(t, func() {
		genErr = genRunWithOptions(genRunOptions{Quiet: true})
	})
	require.ErrorContains(t, genErr, "inspect model columns")

	t.Run("SaysWhyTheInspectionStopped", func(t *testing.T) {
		require.Contains(t, stderr, "panic: gg gen: this function depends on generated column references, which do not exist while gg gen resolves columns")
	})

	t.Run("PointsAtTheSourceLinesInvolved", func(t *testing.T) {
		// Every rewrite keeps its line breaks, so the reported lines are the
		// left-out method and the initialization calling it.
		sourceFile := "model/sample/record.go:"
		require.Contains(t, stderr, "sample.(*Record).statusColumnName(")
		require.Contains(t, stderr, sourceFile+strconv.Itoa(sourceLine(t, source, "func (r *Record) statusColumnName() string {")))
		require.Contains(t, stderr, "sample.init()")
		require.Contains(t, stderr, sourceFile+strconv.Itoa(sourceLine(t, source, "var defaultStatusColumn")))
	})
}

// TestGenRunReportsColumnFilesWrittenBeforeAFailure runs gg gen where writing
// the column files succeeds and removing the stale ones fails, because a
// hand-written file carries the generated suffix. The run fails, but the files
// it already wrote are still reported, and the hand-written file is kept.
func TestGenRunReportsColumnFilesWrittenBeforeAFailure(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	writeProjectFile(t, filepath.Join(projectDir, "model", "sample", "record.go"), `package sample

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	Name string `+"`json:\"name\"`"+`

	model.Base
}

func (Record) TableName() string { return "records" }

func (Record) Design() {
	dsl.Migrate()
}
`)
	handwritten := filepath.Join(projectDir, "model", "sample", "handwritten.gen.go")
	writeProjectFile(t, handwritten, "package sample\n")

	var genErr error
	stdout := captureStdout(t, func() {
		genErr = genRunWithOptions(genRunOptions{})
	})

	require.ErrorContains(t, genErr, "uses the generated file suffix but was not generated by gst; rename it")
	require.Contains(t, stdout, "GENERATE "+filepath.Join("model", "sample", "record.gen.go"))
	require.FileExists(t, handwritten)
}

// TestGenRunRefusesAModelFileNamedLikeAGeneratedFileOfTheModelDirectory pins
// the two file names a model file of the model directory cannot take: its
// column references would be written where the registration file or the
// api doc file goes, replacing it, so gg gen refuses the file by name and
// leaves the registration file as it wrote it. A subpackage's file of the
// same name is a model file like any other, and its column file is cleaned
// up like any other once the source is gone.
func TestGenRunRefusesAModelFileNamedLikeAGeneratedFileOfTheModelDirectory(t *testing.T) {
	note := `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Note struct {
	Name string ` + "`json:\"name\"`" + `

	model.Base
}

func (*Note) TableName() string { return "notes" }

func (Note) Design() {
	dsl.Migrate()
	dsl.Endpoint("notes")
}
`
	t.Run("the registration file's name", func(t *testing.T) {
		projectDir, ok := newGenProject(t)
		if !ok {
			return
		}
		writeProjectFile(t, filepath.Join(projectDir, "model", "model.go"), note)

		require.EqualError(t, genRunWithOptions(genRunOptions{Quiet: true}),
			"the model file model/model.go would get its column references at model/model.gen.go, the registration file; rename the file")
		registration, err := os.ReadFile(filepath.Join("model", "model.gen.go"))
		require.NoError(t, err)
		require.Contains(t, string(registration), "Register[", "the registration file stays what the run wrote")
		require.NotContains(t, string(registration), "NoteCols")
	})

	t.Run("the api doc file's name", func(t *testing.T) {
		projectDir, ok := newGenProject(t)
		if !ok {
			return
		}
		writeProjectFile(t, filepath.Join(projectDir, "model", "apidoc.go"), note)

		require.EqualError(t, genRunWithOptions(genRunOptions{Quiet: true}),
			"the model file model/apidoc.go would get its column references at model/apidoc.gen.go, the api doc file; rename the file")
	})

	t.Run("the same name in a subpackage", func(t *testing.T) {
		projectDir, ok := newGenProject(t)
		if !ok {
			return
		}
		source := filepath.Join(projectDir, "model", "shop", "model.go")
		writeProjectFile(t, source, `package shop

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Item struct {
	Name string `+"`json:\"name\"`"+`

	model.Base
}

func (*Item) TableName() string { return "items" }

func (Item) Design() {
	dsl.Migrate()
	dsl.Endpoint("items")
}
`)
		require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))
		columns, err := os.ReadFile(filepath.Join("model", "shop", "model.gen.go"))
		require.NoError(t, err)
		require.Contains(t, string(columns), "ItemCols")

		require.NoError(t, os.Remove(source))
		require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))
		_, err = os.Stat(filepath.Join("model", "shop", "model.gen.go"))
		require.ErrorIs(t, err, os.ErrNotExist, "the column file goes with its source")
	})
}
