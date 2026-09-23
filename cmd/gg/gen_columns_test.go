package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/hydroan/gst/internal/codegen/gen/columns"
	"github.com/stretchr/testify/require"
)

// TestGenRunGeneratesColumnsReadByHandwrittenModelCode runs gg gen against a
// project whose handwritten model code reads generated column references: a
// hook, package-level vars read by an init function and by other functions,
// and a hook of another model package. The inspection build compiles that
// code before the run writes the references, first with no column file at all
// and then with the previous generation stubbed out; the references the run
// writes must then satisfy the same code in the project's own build.
func TestGenRunGeneratesColumnsReadByHandwrittenModelCode(t *testing.T) {
	projectDir := newGenProject(t)
	writeProjectFile(t, filepath.Join(projectDir, "model", "sample", "record.go"), `package sample

import (
	"context"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	Status string `+"`json:\"status\"`"+`
	Score  int64  `+"`json:\"score\"`"+`

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

	cacheDir, err := columns.CacheDir()
	require.NoError(t, err)
	recordColumnsFile := filepath.Join("model", "sample", "record.gen.go")
	itemColumnsFile := filepath.Join("model", "item", "item.gen.go")

	// First run: no column file exists yet, so nothing declares the
	// references the handwritten code reads.
	require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))
	recordColumns, err := os.ReadFile(recordColumnsFile)
	require.NoError(t, err)
	require.Contains(t, string(recordColumns), "var RecordCols = struct")
	itemColumns, err := os.ReadFile(itemColumnsFile)
	require.NoError(t, err)
	require.Contains(t, string(itemColumns), "var ItemCols = struct")

	// The written references satisfy the handwritten readers.
	output, err := exec.Command("go", "build", "-mod=mod", "./...").CombinedOutput()
	require.NoError(t, err, "go build:\n%s", output)

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
	projectDir := newGenProject(t)
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

	// The project only builds when every import the generated file carries is
	// one it can reach.
	output, err := exec.Command("go", "build", "-mod=mod", "./...").CombinedOutput()
	require.NoError(t, err, "go build:\n%s", output)
}

// TestGenRunReportsCodeReachingAColumnInspectionPlaceholder runs gg gen against
// a project whose package initialization calls a method that reads generated
// column references. Methods are never left out by name, so the call stays in
// the inspection build and reaches the placeholder that replaced the method
// body: generation fails, and what the terminal shows has to say why and point
// at the source lines involved.
func TestGenRunReportsCodeReachingAColumnInspectionPlaceholder(t *testing.T) {
	projectDir := newGenProject(t)
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
	projectDir := newGenProject(t)
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
