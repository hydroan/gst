package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/codegen/constants"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
)

func TestColumnInspectionOverlayStubsGeneratedColumnFiles(t *testing.T) {
	t.Chdir(t.TempDir())
	columns := filepath.Join("model", "sample", "record.gen.go")
	writeCheckFile(t, columns, consts.CodeGeneratedComment()+"\n// source: model/sample/record.go\n\npackage sample\n\nvar RecordCols = struct{}{}\n")
	writeCheckFile(t, filepath.Join("model", "sample", "handwritten.gen.go"), "package sample\n")
	writeCheckFile(t, filepath.Join("model", constants.FileModelGen), consts.CodeGeneratedComment()+"\n\npackage model\n")
	writeCheckFile(t, filepath.Join("model", "sample", "record.go"), "package sample\n\ntype Record struct{}\n")

	overlay, err := columnInspectionOverlay("tmpapp", "model", nil)
	require.NoError(t, err)

	// Only the framework-owned column file collapses to its package clause:
	// the inspection build must not depend on previously generated column
	// references, while every other file, including a hand-written one that
	// merely carries the generated suffix, keeps participating as written.
	require.Equal(t, map[string]string{columns: "package sample\n"}, overlay)
}

// TestColumnInspectionOverlayLeavesOutColumnDependents builds the overlay for
// model packages that read column references no file declares, the state the
// inspection build starts from, and compiles them through it.
func TestColumnInspectionOverlayLeavesOutColumnDependents(t *testing.T) {
	t.Chdir(t.TempDir())
	writeCheckFile(t, "go.mod", "module tmpapp\n\ngo 1.27\n")
	recordSource := filepath.Join("model", "sample", "record.go")
	compactSource := filepath.Join("model", "sample", "compact.go")
	itemSource := filepath.Join("model", "item", "item.go")
	labelSource := filepath.Join("model", "item", "label.go")
	archiveSource := filepath.Join("model", "archive", "archive.go")
	sources := map[string]string{
		recordSource: `package sample

import (
	"context"
	"math/rand/v2"
	"strings"

	format "fmt"
)

type Record struct {
	Status string
}

func (Record) TableName() string { return "records" }

var (
	defaultStatus = "active"
	statusColumn  = RecordCols.Status
)

var statusLabel = strings.ToUpper(statusColumn.Name())

func init() {
	format.Println(statusLabel)
}

func (r *Record) CreateBefore(ctx context.Context) error {
	r.Status = RecordCols.Status.Name()
	return ctx.Err()
}

func pickLabel() string {
	return statusLabel[:rand.IntN(len(statusLabel))]
}

func fallbackStatus() string {
	return defaultStatus
}
`,
		compactSource: `package sample

var compactColumn = RecordCols.Status; var compactFallback = "none"

var (compactFirst = 1; compactStatus = RecordCols.Status; compactLast = 2)
`,
		itemSource: `package item

import "tmpapp/model/sample"

func statusOf(record sample.Record) string {
	return record.Status + sample.RecordCols.Status.Name()
}
`,
		labelSource: `package item

import (
	. "strings"

	. "tmpapp/model/sample"
)

func label() string {
	return ToUpper(RecordCols.Status.Name())
}

func lower(value string) string {
	return ToLower(value)
}
`,
		archiveSource: `package archive

func archiveID() string {
	return ArchiveCols.ID
}
`,
	}
	for path, content := range sources {
		writeCheckFile(t, path, content)
	}
	// Archive was renamed since the previous generation, whose column file is
	// the only place still declaring its column var.
	archiveColumns := filepath.Join("model", "archive", "archive.gen.go")
	writeCheckFile(t, archiveColumns, consts.CodeGeneratedComment()+"\n\npackage archive\n\nvar ArchiveCols = struct{ ID string }{ID: \"id\"}\n")
	models := []*gen.ModelInfo{{
		ModulePath: "tmpapp", ModelPkgName: "sample", ModelName: "Record",
		ModelFileDir: filepath.Join("model", "sample"), ModelFilePath: recordSource,
	}}

	overlay, err := columnInspectionOverlay("tmpapp", "model", models)
	require.NoError(t, err)
	record := overlay[recordSource]

	t.Run("CompilesWithoutTheReferences", func(t *testing.T) {
		overlayFile, err := writeOverlayFile(t.TempDir(), overlay)
		require.NoError(t, err)
		output, err := exec.Command("go", "build", "-overlay", overlayFile, "./model/...").CombinedOutput()
		require.NoError(t, err, "go build:\n%s", output)
	})

	t.Run("KeepsSignaturesAndLeavesOutBodies", func(t *testing.T) {
		require.Contains(t, record, "func (r *Record) CreateBefore(ctx context.Context) error {"+columnInspectionPlaceholder+"\n\n\n}")
		require.Contains(t, record, "func pickLabel() string {"+columnInspectionPlaceholder+"\n\n}")
		// The runtime runs every init, so an init body is emptied instead of
		// being made to panic.
		require.Contains(t, record, "func init() {\n\n}")
		require.Contains(t, record, "func fallbackStatus() string {\n\treturn defaultStatus\n}")
		require.Contains(t, overlay[itemSource], "func statusOf(record sample.Record) string {"+columnInspectionPlaceholder+"\n\n}")
		require.Contains(t, overlay[labelSource], "func label() string {"+columnInspectionPlaceholder+"\n\n}")
	})

	t.Run("DropsVarsAndWhatReadsThem", func(t *testing.T) {
		require.Contains(t, record, `defaultStatus = "active"`)
		require.NotContains(t, record, "statusColumn")
		require.NotContains(t, record, "statusLabel")
		compact := overlay[compactSource]
		require.Contains(t, compact, `var compactFallback = "none"`)
		require.Contains(t, compact, "compactFirst = 1;")
		require.Contains(t, compact, "compactLast = 2)")
		require.NotContains(t, compact, "RecordCols")
	})

	t.Run("BlanksImportsOnlyLeftOutCodeUsed", func(t *testing.T) {
		// The kept signature of CreateBefore still uses context.
		require.Contains(t, record, "\t\"context\"\n")
		// math/rand/v2 is referred to as rand, a name only the go command
		// can tell from the path.
		require.Contains(t, record, "\t_ \"math/rand/v2\"\n")
		require.Contains(t, record, "\t_ \"strings\"\n")
		require.Contains(t, record, "\t_ \"fmt\"\n")
		require.Contains(t, overlay[itemSource], "import \"tmpapp/model/sample\"\n")
		require.Contains(t, overlay[labelSource], "\t. \"strings\"\n")
		require.Contains(t, overlay[labelSource], "\t_ \"tmpapp/model/sample\"\n")
	})

	t.Run("OmitsNamesThePreviousGenerationDeclared", func(t *testing.T) {
		require.Equal(t, "package archive\n", overlay[archiveColumns])
		require.Contains(t, overlay[archiveSource], "func archiveID() string {"+columnInspectionPlaceholder+"\n\n}")
	})

	t.Run("KeepsLineBreaks", func(t *testing.T) {
		for path, content := range sources {
			rewritten, ok := overlay[path]
			require.True(t, ok, "%s depends on column references and must be rewritten", path)
			require.Equal(t, strings.Count(content, "\n"), strings.Count(rewritten, "\n"), path)
		}
	})
}

func TestColumnInspectionOverlayLeavesUnparsableFilesToTheCompiler(t *testing.T) {
	t.Chdir(t.TempDir())
	writeCheckFile(t, filepath.Join("model", "sample", "record.go"), "package sample\n\nfunc status() string {\n\treturn RecordCols.Status.Name()\n")
	models := []*gen.ModelInfo{{
		ModulePath: "tmpapp", ModelPkgName: "sample", ModelName: "Record",
		ModelFileDir: filepath.Join("model", "sample"),
	}}

	overlay, err := columnInspectionOverlay("tmpapp", "model", models)
	require.NoError(t, err)
	require.Empty(t, overlay, "a file that does not parse is compiled as written, so the compiler reports it")
}
