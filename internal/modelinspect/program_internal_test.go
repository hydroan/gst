package modelinspect

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/modelinfo"
	"github.com/stretchr/testify/require"
)

// TestBuildProjectFile pins the file generated for the project next to the
// embedded inspection program: the registration import, one deterministic
// alias per model package, and the models beyond the registry by group, each
// set in init for the variables main.go declares.
func TestBuildProjectFile(t *testing.T) {
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

	file, err := buildProjectFile("tmpapp", all)
	require.NoError(t, err)

	t.Run("ImportsAndListsTheModelsBeyondTheRegistry", func(t *testing.T) {
		// The registration import stays blank while the entries need a named
		// alias, so the root model package is imported twice. A model that
		// declares a Design but no Migrate is listed for the capability guard
		// main.go applies; a table-backed one is listed by name as well, so
		// that gg check validates its index declarations before gg gen wrote
		// the registration.
		require.Equal(t, ggconst.CodeGeneratedComment+"\n\n"+`package main

import (
	_ "tmpapp/model"
	vm0 "tmpapp/model"
	vm1 "tmpapp/model/report"
	vm2 "tmpapp/model/sample"
)

func init() {
	unregisteredModels = []any{&vm0.Note{}, &vm1.Summary{}, &vm1.Trend{}}
	tableBackedModels = []any{&vm2.Record{}}
}
`, file)
	})

	t.Run("CompletesTheEmbeddedProgram", func(t *testing.T) {
		// The embedded main.go declares the variables the file sets, and
		// takes the one value that differs from run to run, the result path,
		// as its argument: a source holding it would be linked and cached
		// anew on every run instead of reusing the program Go already built.
		files := gghelper.InspectProgram()
		files[projectFileName] = file
		for name, source := range files {
			_, err := parser.ParseFile(token.NewFileSet(), name, source, 0)
			require.NoError(t, err, name)
			require.NotContains(t, source, "{{", name)
		}
		require.Contains(t, files["main.go"], "unregisteredModels []any")
		require.Contains(t, files["main.go"], "os.WriteFile(os.Args[1], encoded, 0o600)")
	})

	t.Run("IsDeterministic", func(t *testing.T) {
		again, err := buildProjectFile("tmpapp", all)
		require.NoError(t, err)
		require.Equal(t, file, again)
	})

	t.Run("ListsALoneTableBackedModelByName", func(t *testing.T) {
		// With every model registered, the file still carries the
		// table-backed entries: nothing else.
		bare, err := buildProjectFile("tmpapp", []*modelinfo.Model{registered})
		require.NoError(t, err)
		require.Equal(t, ggconst.CodeGeneratedComment+"\n\n"+`package main

import (
	_ "tmpapp/model"
	vm0 "tmpapp/model/sample"
)

func init() {
	tableBackedModels = []any{&vm0.Record{}}
}
`, bare)
	})
}

// TestBuildProjectFileListsIgnoredModelsApart pins the group of the models
// whose registration gst.yaml gen.models.ignore drops: they stay
// table-backed, so main.go inspects them unconditionally rather than behind
// the capability guard. The file is the example of buildProjectFile's doc
// comment.
func TestBuildProjectFileListsIgnoredModelsApart(t *testing.T) {
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

	file, err := buildProjectFile("tmpapp", []*modelinfo.Model{ignored, virtual, registered})
	require.NoError(t, err)

	require.Equal(t, ggconst.CodeGeneratedComment+"\n\n"+`package main

import (
	_ "tmpapp/model"
	vm0 "tmpapp/model/iam/user"
	vm1 "tmpapp/model/report"
	vm2 "tmpapp/model/sample"
)

func init() {
	unregisteredModels = []any{&vm1.Summary{}}
	ignoredModels = []any{&vm0.User{}}
	tableBackedModels = []any{&vm2.Record{}}
}
`, file)
}
