package main

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen/constants"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/types/consts"
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
		ModulePath: "tmpapp", ModelPkgName: "stats", ModelName: "Summary",
		ModelFileDir: "model/stats", ModelFilePath: "model/stats/summary.go",
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
		require.Contains(t, rendered, `types.NewColumn[*Record, string]("id")`)
		require.Contains(t, rendered, `types.NewColumn[*Record, RecordStatus]("status")`)
	})

	t.Run("SpecializesNumericColumns", func(t *testing.T) {
		// SUM and AVG only belong on a numeric column, because a database
		// answers SUM over text with 0 rather than an error. A named numeric
		// type keeps its own name as the type argument.
		require.Contains(t, rendered, `types.NewNumericColumn[*Record, int64]("amount")`)
		require.Contains(t, rendered, `types.NewNumericColumn[*Record, RecordScore]("score")`)
	})

	t.Run("SpecializesTimeColumns", func(t *testing.T) {
		require.Contains(t, rendered, `types.NewTimeColumn[*Record]("created_at")`)
	})

	t.Run("DegradesUnreproducibleTypesToAny", func(t *testing.T) {
		// A generic instantiation cannot be written back as source, so the
		// column keeps its exact name but loses the value type. The original
		// type is recorded in a comment.
		require.Contains(t, rendered, `types.NewColumn[*Record, any]("tags")`)
		require.Contains(t, rendered, "datatypes.JSONSlice[string]")
	})

	t.Run("DoesNotSpecializeWhenTypeIsUnreproducible", func(t *testing.T) {
		// Specializing would need the type as a type argument, and any is not
		// the column's type. The plain reference keeps the column usable; a
		// NumericColumn minted by hand can still sum it.
		require.Contains(t, rendered, `types.NewColumn[*Record, any]("weight")`)
		require.NotContains(t, rendered, "types.NewNumericColumn[any]")
	})

	t.Run("ImportsOnlyWhatItUses", func(t *testing.T) {
		require.Contains(t, rendered, `types "github.com/hydroan/gst/types"`)
		// The time column renders as NewTimeColumn without a type argument,
		// so the file no longer references time.Time; emitting the import
		// anyway would be an unused import that fails to compile.
		require.NotContains(t, rendered, `time "time"`)
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
	require.Contains(t, rendered, `types.NewNumericColumn[*Record, time.Duration]("elapsed")`)
	require.Contains(t, rendered, `time "time"`)
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

func TestRemoveOrphanColumnFiles(t *testing.T) {
	dir := t.TempDir()
	sampleDir := filepath.Join(dir, "model", "sample")
	require.NoError(t, os.MkdirAll(sampleDir, 0o750))

	generated := consts.CodeGeneratedComment() + "\n\npackage sample\n"
	kept := filepath.Join(sampleDir, "record.gen.go")
	orphan := filepath.Join(sampleDir, "removed.gen.go")
	registration := filepath.Join(dir, "model", constants.FileModelGen)
	require.NoError(t, os.WriteFile(kept, []byte(generated), 0o600))
	require.NoError(t, os.WriteFile(orphan, []byte(generated), 0o600))
	require.NoError(t, os.WriteFile(registration, []byte(generated), 0o600))

	wanted := map[string]struct{}{kept: {}}
	require.NoError(t, removeOrphanColumnFiles(filepath.Join(dir, "model"), wanted, true))

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

	err := removeOrphanColumnFiles(scanDir, map[string]struct{}{}, true)
	require.Error(t, err, "a file without the generated header must not be deleted")
	require.FileExists(t, intruder)
}

// TestGenRunGeneratesColumnsReadByHandwrittenModelCode runs gg gen against a
// project whose handwritten model code reads generated column references: a
// hook, package-level vars read by an init function and by other functions,
// and a hook of another model package. The inspection build compiles that
// code before the run writes the references, first with no column file at all
// and then with the previous generation stubbed out; the references the run
// writes must then satisfy the same code in the project's own build.
func TestGenRunGeneratesColumnsReadByHandwrittenModelCode(t *testing.T) {
	projectDir := newGenProject(t)
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "record.go"), `package sample

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
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "status.go"), `package sample

import (
	"math/rand/v2"
	"strings"

	"github.com/hydroan/gst/types"
)

var (
	defaultStatus = "active"
	statusColumn  = RecordCols.Status
)

var sortableColumns = []types.AnyColumnRef{statusColumn, RecordCols.Score}

func init() {
	if strings.TrimSpace(statusColumn.Name()) == "" {
		panic("status column has no name")
	}
}

func statusFilter() types.Filter {
	return statusColumn.Eq(defaultStatus + strings.Repeat("!", rand.IntN(2)))
}

func sortColumns() []types.AnyColumnRef {
	return sortableColumns
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "model", "item", "item.go"), `package item

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

	cacheDir, err := columnsCacheDir()
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
