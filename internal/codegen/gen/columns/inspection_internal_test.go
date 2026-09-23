package columns

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/stretchr/testify/require"
)

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
		// The builder fills every placeholder, and the program takes the one
		// value that differs from run to run, the result path, as its
		// argument: a source holding it would be linked and cached anew on
		// every run instead of reusing the program Go already built.
		require.NotContains(t, program, "{{")
		require.Contains(t, program, "os.WriteFile(os.Args[1], encoded, 0o600)")
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

	t.Run("FillsTheTemplateWithAliasedImportsAndModelEntries", func(t *testing.T) {
		// The imports and entries are the example of buildColumnsProgram's doc
		// comment.
		imports := "\tvm0 \"tmpapp/model/iam/user\"\n\tvm1 \"tmpapp/model/report\"\n"
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
`
		want := strings.NewReplacer("{{MODULE}}", "tmpapp", "{{UNREGISTERED_IMPORTS}}", imports, "{{UNREGISTERED_MODELS}}", entries).Replace(columnsProgram)
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
		alone := buildColumnsProgram("tmpapp", []*gen.ModelInfo{ignored})
		require.Contains(t, alone, "models = append(models,\n\t\t&vm0.User{},\n\t)")
		_, err := parser.ParseFile(token.NewFileSet(), "main.go", alone, 0)
		require.NoError(t, err)
	})
}
