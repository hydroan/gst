package modelinspect

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/modelinfo"
	"github.com/stretchr/testify/require"
)

func TestBuildProgram(t *testing.T) {
	registered := &modelinfo.Model{
		ModulePath: "tmpapp", ModelPkgName: "sample", ModelName: "Record",
		ModelFileDir: "model/sample", Design: &dsl.Design{Migrate: true},
	}
	summary := &modelinfo.Model{
		ModulePath: "tmpapp", ModelPkgName: "report", ModelName: "Summary",
		ModelFileDir: "model/report", Design: &dsl.Design{},
	}
	trend := &modelinfo.Model{
		ModulePath: "tmpapp", ModelPkgName: "report", ModelName: "Trend",
		ModelFileDir: "model/report", Design: &dsl.Design{},
	}
	note := &modelinfo.Model{
		ModulePath: "tmpapp", ModelPkgName: "model", ModelName: "Note",
		ModelFileDir: "model", Design: &dsl.Design{},
	}
	all := []*modelinfo.Model{registered, summary, trend, note}

	program := buildProgram("tmpapp", all)

	t.Run("ValidatesTheIndexDeclarationsOfEveryTableBackedModel", func(t *testing.T) {
		// The declarations are validated against a MySQL handle that never
		// connects, and a table-backed model is compiled in by name as well:
		// the registry lists it once gg gen wrote model.gen.go, and gg check
		// validates it before that too.
		require.Contains(t, program, "schemaDB, err := gstmysql.DryRun()")
		require.Contains(t, program, "model.ValidateIndexes(schemaDB, m)")
		require.Contains(t, program, "\t\t&vm2.Record{},\n")
	})

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

	t.Run("CompilesTableBackedModelsInByName", func(t *testing.T) {
		// A migrated model arrives through model.RegisteredModels once gg gen
		// wrote the registration; it is compiled in by name as well so that
		// gg check validates its index declarations before that, and the
		// program inspects a model listed twice once (see seen).
		require.Contains(t, program, `vm2 "tmpapp/model/sample"`)
		require.Contains(t, program, "\tmodels = append(models,\n\t\t&vm2.Record{},\n\t)\n")
	})

	t.Run("ProducesParseableSource", func(t *testing.T) {
		_, err := parser.ParseFile(token.NewFileSet(), "main.go", program, 0)
		require.NoError(t, err)
		// The builder fills every placeholder, and the program takes the one
		// value that differs from run to run, the result path, as its
		// argument: a source holding it would be linked and cached anew on
		// every run instead of reusing the program Go already built.
		require.NotContains(t, program, "{{")
		require.Contains(t, program, "os.WriteFile(os.Args[1], encoded, 0o600)")
	})

	t.Run("IsDeterministic", func(t *testing.T) {
		require.Equal(t, program, buildProgram("tmpapp", all))
	})

	t.Run("CompilesALoneTableBackedModelInByName", func(t *testing.T) {
		// With every model registered, the program still carries the
		// table-backed entries: nothing else, and no capability guard.
		bare := buildProgram("tmpapp", []*modelinfo.Model{registered})
		entries := `	// Table-backed models are compiled in by name as well: the registry
	// lists them once gg gen wrote model.gen.go, and gg check validates their
	// index declarations before that too. One the registry already lists is
	// inspected once (see seen).
	models = append(models,
		&vm0.Record{},
	)
`
		template := strings.NewReplacer("{{MODULE}}", "tmpapp", "{{UNREGISTERED_IMPORTS}}", "\tvm0 \"tmpapp/model/sample\"\n", "{{UNREGISTERED_MODELS}}", entries).Replace(inspectionProgram)
		require.Equal(t, template, bare)
		_, err := parser.ParseFile(token.NewFileSet(), "main.go", bare, 0)
		require.NoError(t, err)
	})
}

func TestBuildProgramInspectsIgnoredModelsUnconditionally(t *testing.T) {
	ignored := &modelinfo.Model{
		ModulePath: "tmpapp", ModelPkgName: "user", ModelName: "User",
		ModelFileDir: "model/iam/user", ModelFilePath: "model/iam/user/user.go",
		Design:          &dsl.Design{Migrate: false},
		RegisterIgnored: true,
	}
	virtual := &modelinfo.Model{
		ModulePath: "tmpapp", ModelPkgName: "report", ModelName: "Summary",
		ModelFileDir: "model/report", ModelFilePath: "model/report/summary.go",
		Design: &dsl.Design{Migrate: false},
	}
	registered := &modelinfo.Model{
		ModulePath: "tmpapp", ModelPkgName: "sample", ModelName: "Record",
		ModelFileDir: "model/sample", ModelFilePath: "model/sample/record.go",
		Design: &dsl.Design{Migrate: true},
	}

	program := buildProgram("tmpapp", []*modelinfo.Model{ignored, virtual, registered})

	t.Run("FillsTheTemplateWithAliasedImportsAndModelEntries", func(t *testing.T) {
		// The imports and entries are the example of buildProgram's doc
		// comment.
		imports := "\tvm0 \"tmpapp/model/iam/user\"\n\tvm1 \"tmpapp/model/report\"\n\tvm2 \"tmpapp/model/sample\"\n"
		entries := `	// Models that declare a Design but no Migrate never reach the registry.
	// Their query columns resolve the same way, so those that opted in to
	// framework query parameters are inspected alongside the registered ones.
	for _, m := range []any{
		&vm1.Summary{},
	} {
		if !modelschema.IsQueryable(m) {
			continue
		}
		models = append(models, m)
	}
	// Models whose registration is ignored by gst.yaml gen.models.ignore
	// stay table-backed: their column files must keep matching the
	// module-copied model sources, so they are inspected unconditionally.
	models = append(models,
		&vm0.User{},
	)
	// Table-backed models are compiled in by name as well: the registry
	// lists them once gg gen wrote model.gen.go, and gg check validates their
	// index declarations before that too. One the registry already lists is
	// inspected once (see seen).
	models = append(models,
		&vm2.Record{},
	)
`
		want := strings.NewReplacer("{{MODULE}}", "tmpapp", "{{UNREGISTERED_IMPORTS}}", imports, "{{UNREGISTERED_MODELS}}", entries).Replace(inspectionProgram)
		require.Equal(t, want, program)
	})

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
		alone := buildProgram("tmpapp", []*modelinfo.Model{ignored})
		require.Contains(t, alone, "models = append(models,\n\t\t&vm0.User{},\n\t)")
		_, err := parser.ParseFile(token.NewFileSet(), "main.go", alone, 0)
		require.NoError(t, err)
	})
}
