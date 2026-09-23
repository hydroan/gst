package columns

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/stretchr/testify/require"
)

func TestColumnsFileName(t *testing.T) {
	require.Equal(t, "model/sample/record.gen.go", columnsFileName("model/sample/record.go"))
	require.Equal(t, "model/record.gen.go", columnsFileName("model/record.go"))
}

func TestModelPkgPath(t *testing.T) {
	require.Equal(t, "tmpapp/model/sample",
		modelPkgPath(&gen.ModelInfo{ModulePath: "tmpapp", ModelFileDir: "model/sample"}))
	require.Equal(t, "tmpapp/model",
		modelPkgPath(&gen.ModelInfo{ModulePath: "tmpapp", ModelFileDir: "model/"}))
	require.Equal(t, "tmpapp",
		modelPkgPath(&gen.ModelInfo{ModulePath: "tmpapp", ModelFileDir: ""}))
}

func TestBuildColumnsProgram(t *testing.T) {
	registered := &gen.ModelInfo{
		ModulePath: "tmpapp", ModelPkgName: "sample", ModelName: "Record",
		ModelFileDir: "model/sample", Design: &dsl.Design{Enabled: true, Migrate: true},
	}
	summary := &gen.ModelInfo{
		ModulePath: "tmpapp", ModelPkgName: "report", ModelName: "Summary",
		ModelFileDir: "model/report", Design: &dsl.Design{Enabled: true},
	}
	trend := &gen.ModelInfo{
		ModulePath: "tmpapp", ModelPkgName: "report", ModelName: "Trend",
		ModelFileDir: "model/report", Design: &dsl.Design{Enabled: true},
	}
	disabled := &gen.ModelInfo{
		ModulePath: "tmpapp", ModelPkgName: "draft", ModelName: "Draft",
		ModelFileDir: "model/draft", Design: &dsl.Design{Enabled: false},
	}
	note := &gen.ModelInfo{
		ModulePath: "tmpapp", ModelPkgName: "model", ModelName: "Note",
		ModelFileDir: "model", Design: &dsl.Design{Enabled: true},
	}
	all := []*gen.ModelInfo{registered, summary, trend, disabled, note}

	program := buildColumnsProgram("tmpapp", all)

	t.Run("EnumeratesUnregisteredModelsBehindCapabilityGuard", func(t *testing.T) {
		// A model that declares a Design but no Migrate never reaches the
		// runtime registry, so the program carries it as an explicit entry;
		// the guard keeps only the models that opted in to framework query
		// parameters, which is what gives them a filter and sort column
		// namespace worth generating references for.
		require.Contains(t, program, `vm1 "tmpapp/model/report"`)
		require.Contains(t, program, "&vm1.Summary{},")
		require.Contains(t, program, "&vm1.Trend{},")
		require.Contains(t, program, "modelschema.IsQueryable")
	})

	t.Run("ImportsRootPackageModelsUnderTheirOwnAlias", func(t *testing.T) {
		// The registration import must stay blank while the enumeration needs
		// a named alias, so the root model package is imported twice.
		require.Contains(t, program, `_ "tmpapp/model"`)
		require.Contains(t, program, `vm0 "tmpapp/model"`)
		require.Contains(t, program, "&vm0.Note{},")
	})

	t.Run("LeavesRegisteredAndDisabledModelsToTheRegistry", func(t *testing.T) {
		// A migrated model arrives through model.RegisteredModels and a
		// disabled Design is not part of the API; enumerating either would
		// resurrect it behind the registry's back.
		require.NotContains(t, program, "model/sample")
		require.NotContains(t, program, "model/draft")
	})

	t.Run("ProducesParseableSource", func(t *testing.T) {
		_, err := parser.ParseFile(token.NewFileSet(), "main.go", program, 0)
		require.NoError(t, err)
		// The builder owns three placeholders; {{OUTPUT}} stays for
		// inspectColumns to fill on each run.
		require.NotContains(t, program, "{{MODULE}}")
		require.NotContains(t, program, "{{UNREGISTERED_IMPORTS}}")
		require.NotContains(t, program, "{{UNREGISTERED_MODELS}}")
		require.Contains(t, program, "{{OUTPUT}}")
	})

	t.Run("IsDeterministic", func(t *testing.T) {
		require.Equal(t, program, buildColumnsProgram("tmpapp", all))
	})

	t.Run("LeavesEnumerationToTheRegistryWhenEveryModelIsRegistered", func(t *testing.T) {
		// The registry already enumerates every migrated model, so nothing is
		// compiled in beyond the template itself.
		bare := buildColumnsProgram("tmpapp", []*gen.ModelInfo{registered})
		template := strings.NewReplacer("{{MODULE}}", "tmpapp", "{{UNREGISTERED_IMPORTS}}", "", "{{UNREGISTERED_MODELS}}", "").Replace(columnsProgram)
		require.Equal(t, template, bare)
		_, err := parser.ParseFile(token.NewFileSet(), "main.go", bare, 0)
		require.NoError(t, err)
	})
}

func TestBuildColumnsProgramInspectsIgnoredModelsUnconditionally(t *testing.T) {
	ignored := &gen.ModelInfo{
		ModulePath: "tmpapp", ModelPkgName: "user", ModelName: "User",
		ModelFileDir: "model/iam/user", ModelFilePath: "model/iam/user/user.go",
		Design:          &dsl.Design{Enabled: true, Migrate: false},
		RegisterIgnored: true,
	}
	virtual := &gen.ModelInfo{
		ModulePath: "tmpapp", ModelPkgName: "report", ModelName: "Summary",
		ModelFileDir: "model/report", ModelFilePath: "model/report/summary.go",
		Design: &dsl.Design{Enabled: true, Migrate: false},
	}

	program := buildColumnsProgram("tmpapp", []*gen.ModelInfo{ignored, virtual})

	t.Run("AppendsIgnoredModelsWithoutTheCapabilityGuard", func(t *testing.T) {
		// A model ignored by gst.yaml gen.models.ignore stays table-backed:
		// its column file must keep matching the module-copied source, so it
		// is inspected unconditionally instead of behind IsQueryable.
		require.Contains(t, program, "models = append(models,\n\t\t&vm0.User{},\n\t)")
	})

	t.Run("KeepsTheCapabilityGuardForVirtualModels", func(t *testing.T) {
		require.Contains(t, program, "&vm1.Summary{},")
		require.Contains(t, program, "modelschema.IsQueryable")
	})

	t.Run("ProducesParseableSource", func(t *testing.T) {
		_, err := parser.ParseFile(token.NewFileSet(), "main.go", program, 0)
		require.NoError(t, err)
	})

	t.Run("ProducesParseableSourceForIgnoredModelsAlone", func(t *testing.T) {
		alone := buildColumnsProgram("tmpapp", []*gen.ModelInfo{ignored})
		require.Contains(t, alone, "models = append(models,\n\t\t&vm0.User{},\n\t)")
		_, err := parser.ParseFile(token.NewFileSet(), "main.go", alone, 0)
		require.NoError(t, err)
	})
}

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

func TestRenderColumnsFile(t *testing.T) {
	models := []modelColumns{{
		PkgPath: "tmpapp/model/sample",
		PkgName: "sample",
		Name:    "Record",
		Columns: []columnInfo{
			{GoName: "Amount", DBName: "amount", TypeExpr: "int64", TypeName: "int64", Numeric: true},
			{GoName: "CreatedAt", DBName: "created_at", TypeExpr: "time.Time", TypePkg: "time", TypeName: "time.Time", Time: true},
			{GoName: "ID", DBName: "id", TypeExpr: "string", TypeName: "string"},
			{GoName: "Score", DBName: "score", TypeExpr: "RecordScore", TypeName: "sample.RecordScore", Numeric: true},
			{GoName: "Status", DBName: "status", TypeExpr: "RecordStatus", TypeName: "sample.RecordStatus"},
			{GoName: "Tags", DBName: "tags", TypeName: "datatypes.JSONSlice[string]"},
			{GoName: "Weight", DBName: "weight", TypeName: "weird.Numeric[int]", Numeric: true},
		},
	}}

	rendered, err := renderColumnsFile("tmpapp", "sample", "model/sample/record.go", models)
	require.NoError(t, err)

	t.Run("CarriesGeneratedHeaderAndSource", func(t *testing.T) {
		require.True(t, strings.HasPrefix(rendered, "// Code generated by gst; DO NOT EDIT."))
		require.Contains(t, rendered, "// source: model/sample/record.go")
		require.Contains(t, rendered, "package sample")
	})

	t.Run("DeclaresTypedColumns", func(t *testing.T) {
		require.Contains(t, rendered, `gst.NewColumn[*Record, string]("id")`)
		require.Contains(t, rendered, `gst.NewColumn[*Record, RecordStatus]("status")`)
	})

	t.Run("SpecializesNumericColumns", func(t *testing.T) {
		// SUM and AVG only belong on a numeric column, because a database
		// answers SUM over text with 0 rather than an error. A named numeric
		// type keeps its own name as the type argument.
		require.Contains(t, rendered, `gst.NewNumericColumn[*Record, int64]("amount")`)
		require.Contains(t, rendered, `gst.NewNumericColumn[*Record, RecordScore]("score")`)
	})

	t.Run("SpecializesTimeColumns", func(t *testing.T) {
		require.Contains(t, rendered, `gst.NewTimeColumn[*Record]("created_at")`)
	})

	t.Run("DegradesUnreproducibleTypesToAny", func(t *testing.T) {
		// A generic instantiation cannot be written back as source, so the
		// column keeps its exact name but loses the value type. The original
		// type is recorded in a comment.
		require.Contains(t, rendered, `gst.NewColumn[*Record, any]("tags")`)
		require.Contains(t, rendered, "datatypes.JSONSlice[string]")
	})

	t.Run("DoesNotSpecializeWhenTypeIsUnreproducible", func(t *testing.T) {
		// Specializing would need the type as a type argument, and any is not
		// the column's type. The plain reference keeps the column usable; a
		// NumericColumn minted by hand can still sum it.
		require.Contains(t, rendered, `gst.NewColumn[*Record, any]("weight")`)
		require.NotContains(t, rendered, "gst.NewNumericColumn[any]")
	})

	t.Run("ImportsOnlyWhatItUses", func(t *testing.T) {
		require.Contains(t, rendered, `"github.com/hydroan/gst"`)
		// The time column renders as NewTimeColumn without a type argument,
		// so the file no longer references time.Time; emitting the import
		// anyway would be an unused import that fails to compile.
		require.NotContains(t, rendered, "\n\t\"time\"\n")
		require.NotContains(t, rendered, "gorm.io/datatypes")
	})

	t.Run("IsDeterministic", func(t *testing.T) {
		again, err := renderColumnsFile("tmpapp", "sample", "model/sample/record.go", models)
		require.NoError(t, err)
		require.Equal(t, rendered, again)
	})
}

func TestRenderColumnsFileKeepsImportsUsedByTypeArguments(t *testing.T) {
	// time.Duration is numeric, so its reference keeps the type argument and
	// with it the import; only the TimeColumn specialization drops both.
	models := []modelColumns{{
		PkgPath: "tmpapp/model/sample",
		PkgName: "sample",
		Name:    "Record",
		Columns: []columnInfo{
			{GoName: "CreatedAt", DBName: "created_at", TypeExpr: "time.Time", TypePkg: "time", TypeName: "time.Time", Time: true},
			{GoName: "Elapsed", DBName: "elapsed", TypeExpr: "time.Duration", TypePkg: "time", TypeName: "time.Duration", Numeric: true},
		},
	}}

	rendered, err := renderColumnsFile("tmpapp", "sample", "model/sample/record.go", models)
	require.NoError(t, err)
	require.Contains(t, rendered, `gst.NewNumericColumn[*Record, time.Duration]("elapsed")`)
	require.Contains(t, rendered, "import (\n\t\"time\"\n", "the standard library import is kept, with the name left to the path")
}

func TestRenderColumnsFileRejectsImportAliasCollision(t *testing.T) {
	models := []modelColumns{{
		PkgName: "sample",
		Name:    "Record",
		Columns: []columnInfo{
			{GoName: "Left", DBName: "left", TypeExpr: "shared.Kind", TypePkg: "tmpapp/a/shared"},
			{GoName: "Right", DBName: "right", TypeExpr: "shared.Kind", TypePkg: "tmpapp/b/shared"},
		},
	}}

	_, err := renderColumnsFile("tmpapp", "sample", "model/sample/record.go", models)
	require.Error(t, err, "two packages cannot share one import alias")
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

// TestColumnsCacheKeyCoversTheModelCode pins which files the cache key
// hashes: every model source a walk over the project's code reads, a model
// package named generated among them, and none it leaves out, such as a
// testdata directory.
func TestColumnsCacheKeyCoversTheModelCode(t *testing.T) {
	t.Chdir(t.TempDir())
	writeProjectFile(t, "go.mod", "module tmpapp\n\ngo 1.26\n")
	writeProjectFile(t, filepath.Join(ggconst.DirModel, "record.go"), "package model\n")
	generated := filepath.Join(ggconst.DirModel, "generated", "sample.go")
	fixture := filepath.Join(ggconst.DirModel, "testdata", "fixture.go")
	writeProjectFile(t, generated, "package generated\n")
	writeProjectFile(t, fixture, "package fixture\n")
	key := func() string {
		t.Helper()
		k, err := columnsCacheKey("program", ggconst.DirModel, gghelper.NewProjectIgnore())
		require.NoError(t, err)
		return k
	}

	before := key()
	writeProjectFile(t, generated, "package generated\n\ntype Sample struct{}\n")
	require.NotEqual(t, before, key(), "a change to the model package named generated must change the key")

	before = key()
	writeProjectFile(t, fixture, "package fixture\n\ntype Fixture struct{}\n")
	require.Equal(t, before, key(), "a change under testdata must leave the key alone")
}
