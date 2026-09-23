package columns

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/stretchr/testify/require"
)

func TestGroupColumnsByFile(t *testing.T) {
	id := columnInfo{GoName: "ID", DBName: "id", TypeExpr: "string", TypeName: "string"}
	sources := map[string]string{
		"tmpapp/model/sample.Record":  "model/sample/record.go",
		"tmpapp/model/report.Summary": "model/report/summary.go",
	}
	resolved := []modelColumns{
		{PkgPath: "tmpapp/model/sample", PkgName: "sample", Name: "Record", Columns: []columnInfo{id}},
		// Summary resolved no columns: an action-only model would render an
		// empty Cols var that references nothing.
		{PkgPath: "tmpapp/model/report", PkgName: "report", Name: "Summary"},
		// External has no source file in the scan: it was registered from
		// outside the project's model directory, such as a framework module.
		{PkgPath: "github.com/elsewhere/mod", PkgName: "ext", Name: "External", Columns: []columnInfo{id}},
	}

	byFile := groupColumnsByFile(resolved, sources)

	require.Len(t, byFile, 1)
	require.Len(t, byFile["model/sample/record.go"], 1)
	require.Equal(t, "Record", byFile["model/sample/record.go"][0].Name)
}

func TestColumnsFileName(t *testing.T) {
	require.Equal(t, "model/sample/record.gen.go", columnsFileName("model/sample/record.go"))
	require.Equal(t, "model/record.gen.go", columnsFileName("model/record.go"))
}

func TestWriteGeneratedFileIfChanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "record.gen.go")

	written, err := writeGeneratedFileIfChanged(path, "package sample\n")
	require.NoError(t, err)
	require.True(t, written, "a missing file is written")

	written, err = writeGeneratedFileIfChanged(path, "package sample\n")
	require.NoError(t, err)
	require.False(t, written, "a file already holding the content is left alone")

	changed := "package sample\n\nvar RecordCols = struct{}{}\n"
	written, err = writeGeneratedFileIfChanged(path, changed)
	require.NoError(t, err)
	require.True(t, written, "a file whose content changed is rewritten")
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, changed, string(content))
}

func TestRemoveOrphanColumnFiles(t *testing.T) {
	dir := t.TempDir()
	sampleDir := filepath.Join(dir, "model", "sample")
	require.NoError(t, os.MkdirAll(sampleDir, 0o750))

	generated := consts.CodeGeneratedComment() + "\n\npackage sample\n"
	kept := filepath.Join(sampleDir, "record.gen.go")
	orphan := filepath.Join(sampleDir, "removed.gen.go")
	registration := filepath.Join(dir, "model", ggconst.FileModelGen)
	require.NoError(t, os.WriteFile(kept, []byte(generated), 0o600))
	require.NoError(t, os.WriteFile(orphan, []byte(generated), 0o600))
	require.NoError(t, os.WriteFile(registration, []byte(generated), 0o600))

	wanted := map[string]struct{}{kept: {}}
	removed, err := removeOrphanColumnFiles(filepath.Join(dir, "model"), wanted)
	require.NoError(t, err)
	require.Equal(t, []string{orphan}, removed, "the result lists the removed file and nothing else")

	require.FileExists(t, kept, "a file whose model source still exists is kept")
	require.NoFileExists(t, orphan, "a file whose model source is gone is removed")
	require.FileExists(t, registration, "the registration file belongs to another generation step")
}

func TestRemoveOrphanColumnFilesRefusesHandWrittenFile(t *testing.T) {
	dir := t.TempDir()
	scanDir := filepath.Join(dir, "model")
	require.NoError(t, os.MkdirAll(scanDir, 0o750))
	intruder := filepath.Join(scanDir, "handwritten.gen.go")
	require.NoError(t, os.WriteFile(intruder, []byte("package model\n"), 0o600))

	_, err := removeOrphanColumnFiles(scanDir, map[string]struct{}{})
	require.Error(t, err, "a file without the generated header must not be deleted")
	require.FileExists(t, intruder)
}

func TestModelPkgPath(t *testing.T) {
	require.Equal(t, "tmpapp/model/sample",
		modelPkgPath(&gen.ModelInfo{ModulePath: "tmpapp", ModelFileDir: "model/sample"}))
	require.Equal(t, "tmpapp/model",
		modelPkgPath(&gen.ModelInfo{ModulePath: "tmpapp", ModelFileDir: "model/"}))
	require.Equal(t, "tmpapp",
		modelPkgPath(&gen.ModelInfo{ModulePath: "tmpapp", ModelFileDir: ""}))
}
