package jsonshape_test

import (
	"go/types"
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/jsonshape"
	"github.com/stretchr/testify/require"
)

// fixtureModule is the module path the fixture packages stand in for a
// project module under. They are kept here for every generator that reads
// shapes off a project; the TypeScript golden files render these very shapes.
const fixtureModule = "github.com/hydroan/gst/internal/jsonshape/fixture"

// TestLoadReadsTheShapesOfAProject pins what a loaded project answers: the
// packages it holds, the types it declares and their doc comments, the keys a
// struct encodes to in encoding/json's order, and the constants of an enum
// type in source order.
func TestLoadReadsTheShapesOfAProject(t *testing.T) {
	project, err := jsonshape.Load(jsonshape.Config{
		Dir:        ".",
		ModulePath: fixtureModule,
		Roots:      []string{fixtureModule + "/model/sample"},
	})
	require.NoError(t, err)
	require.Nil(t, project.Package("example.com/elsewhere"))

	pkg := project.Package(fixtureModule + "/model/sample")
	require.NotNil(t, pkg)
	sample, ok := pkg.Types.Scope().Lookup("Sample").(*types.TypeName)
	require.True(t, ok)
	require.True(t, project.Declares(sample))
	require.Equal(t, "Sample is a resource kept by the sample service.", project.TypeDoc(sample))

	// The keys of the embedded model base come first, promoted the way
	// encoding/json promotes them, then the keys of the type's own fields.
	st, ok := sample.Type().Underlying().(*types.Struct)
	require.True(t, ok)
	fields := project.Fields(st, jsonshape.Site{Subject: "sample.Sample", Pos: sample.Pos()})
	keys := make([]string, 0, len(fields))
	for _, f := range fields {
		keys = append(keys, f.Key)
	}
	require.Equal(t, []string{"id", "created_by", "updated_by", "created_at", "updated_at", "name"}, keys[:6])
	require.Equal(t, "Name is the display name.", project.FieldDoc(fields[5].Var))

	status, ok := pkg.Types.Scope().Lookup("Status").(*types.TypeName)
	require.True(t, ok)
	enum := project.Enum(status)
	require.NotNil(t, enum)
	require.False(t, enum.Bitwise)
	require.False(t, enum.CoversZero, "no constant of Status is the empty string")
	require.Equal(t, "StatusActive", enum.Values[0].Const.Name())
	require.Equal(t, "StatusActive marks a sample in use.", enum.Values[0].Doc)

	require.Empty(t, project.Diagnostics())
}

// TestLoadReadsADeclaredTreeLikeTheProject pins what a declared tree adds to a
// load: the packages under it the roots reach are type-checked from source
// with the project's, so their types are declared, where a package outside
// the project is otherwise not one the project holds.
func TestLoadReadsADeclaredTreeLikeTheProject(t *testing.T) {
	cfg := jsonshape.Config{Dir: ".", ModulePath: fixtureModule + "/model", Roots: []string{fixtureModule + "/model/sample"}}
	project, err := jsonshape.Load(cfg)
	require.NoError(t, err)
	require.Nil(t, project.Package(fixtureModule+"/pkg/notifier"))

	cfg.DeclaredTrees = []string{fixtureModule + "/pkg"}
	project, err = jsonshape.Load(cfg)
	require.NoError(t, err)
	pkg := project.Package(fixtureModule + "/pkg/notifier")
	require.NotNil(t, pkg)
	endpoint, ok := pkg.Types.Scope().Lookup("Endpoint").(*types.TypeName)
	require.True(t, ok)
	require.True(t, project.Declares(endpoint))
	require.Equal(t, "Endpoint is where a sample reports to.", project.TypeDoc(endpoint))
}

// TestFormatOfNamesTheFormatOfABuiltinString pins the formats of the builtin
// strings, the names the OpenAPI document gives them: date-time for an
// instant, date for the framework's calendar day, and none for its time of
// day, for a builtin of another shape or for a type that is no builtin.
func TestFormatOfNamesTheFormatOfABuiltinString(t *testing.T) {
	project, err := jsonshape.Load(jsonshape.Config{
		Dir:        ".",
		ModulePath: fixtureModule,
		Roots:      []string{fixtureModule + "/model/sample"},
	})
	require.NoError(t, err)
	sample, ok := project.Package(fixtureModule + "/model/sample").Types.Scope().Lookup("Sample").Type().Underlying().(*types.Struct)
	require.True(t, ok)

	formats := make(map[string]string)
	for field := range sample.Fields() {
		if named, isNamed := types.Unalias(field.Type()).(*types.Named); isNamed {
			formats[field.Name()] = jsonshape.FormatOf(named)
		}
	}
	require.Equal(t, "date-time", formats["Deadline"], "time.Time")
	require.Equal(t, "date-time", formats["Archived"], "gorm.DeletedAt")
	require.Equal(t, "date", formats["Due"], "datatypes.Date")
	require.Empty(t, formats["Opens"], "datatypes.TimeOfDay is a string with no format of its own")
	require.Empty(t, formats["Document"], "datatypes.JSON is no string")
	require.Empty(t, formats["Comment"], "sql.NullString is no builtin")
}

// TestTextMethodNamesHowATypeWithoutAJSONMethodWritesText pins which types
// a generator describes as strings: a type with MarshalText and no JSON
// method, netip.Addr and the Grade fixture, writes the text of that method,
// so TextMethod names it and MarshalMethod answers nothing; a type with a
// JSON method, time.Time, writes whatever that method does, so MarshalMethod
// names it and TextMethod answers nothing; a type with neither answers
// nothing for both. The constants of a type with a text method are no enum:
// the method decides what they write.
func TestTextMethodNamesHowATypeWithoutAJSONMethodWritesText(t *testing.T) {
	project, err := jsonshape.Load(jsonshape.Config{
		Dir:        ".",
		ModulePath: fixtureModule,
		Roots:      []string{fixtureModule + "/model/sample"},
	})
	require.NoError(t, err)
	sample, ok := project.Package(fixtureModule + "/model/sample").Types.Scope().Lookup("Sample").(*types.TypeName)
	require.True(t, ok)
	st, ok := sample.Type().Underlying().(*types.Struct)
	require.True(t, ok)
	field := func(name string) types.Type {
		for f := range st.Fields() {
			if f.Name() == name {
				return f.Type()
			}
		}
		t.Fatalf("Sample has no field %s", name)
		return nil
	}
	require.Equal(t, "MarshalText", project.TextMethod(field("Addr")))
	require.Empty(t, project.MarshalMethod(field("Addr")))
	require.Equal(t, "MarshalText", project.TextMethod(field("Grade")))
	require.Empty(t, project.MarshalMethod(field("Grade")))
	require.Empty(t, project.TextMethod(field("Deadline")), "time.Time writes its own JSON")
	require.Equal(t, "MarshalJSON", project.MarshalMethod(field("Deadline")))
	require.Empty(t, project.TextMethod(field("Name")))
	require.Empty(t, project.MarshalMethod(field("Name")))

	grade, ok := project.Package(fixtureModule + "/model/record").Types.Scope().Lookup("Grade").(*types.TypeName)
	require.True(t, ok)
	require.Nil(t, project.Enum(grade), "the constants of a type with a text method are no enum")
	require.Empty(t, project.Diagnostics())
}

// TestLoadAppliesTheOverlay pins that the files of Config.Overlay take part
// in the load in place of, or beside, the ones on disk, keyed by a path
// relative to Config.Dir or an absolute one: a declaration only the overlay
// holds is in the loaded package.
func TestLoadAppliesTheOverlay(t *testing.T) {
	project, err := jsonshape.Load(jsonshape.Config{
		Dir:        ".",
		ModulePath: fixtureModule,
		Roots:      []string{fixtureModule + "/model/sample"},
		Overlay:    map[string][]byte{filepath.Join("fixture", "model", "sample", "overlaid.gen.go"): []byte("package sample\n\ntype Overlaid struct{}\n")},
	})
	require.NoError(t, err)

	pkg := project.Package(fixtureModule + "/model/sample")
	require.NotNil(t, pkg)
	require.NotNil(t, pkg.Types.Scope().Lookup("Overlaid"), "the overlaid file must be part of the load")
}

// TestLoadResolvesThroughTheWorkspace pins that a project in a go workspace
// is read the way go build reads it there: a package a workspace module
// provides, which the project's go.mod does not require, type-checks.
func TestLoadResolvesThroughTheWorkspace(t *testing.T) {
	dir := t.TempDir()
	appDir := filepath.Join(dir, "app")
	writeFile(t, filepath.Join(appDir, "go.mod"), "module example.com/app\n\ngo 1.27\n")
	writeFile(t, filepath.Join(appDir, "model", "item.go"), "package model\n\nimport \"example.com/lib\"\n\n// Item is a sample resource.\ntype Item struct {\n\tName lib.Name `json:\"name\"`\n}\n")
	writeFile(t, filepath.Join(dir, "lib", "go.mod"), "module example.com/lib\n\ngo 1.27\n")
	writeFile(t, filepath.Join(dir, "lib", "lib.go"), "package lib\n\n// Name is a display name.\ntype Name string\n")
	goWork := filepath.Join(dir, "go.work")
	writeFile(t, goWork, "go 1.27\n\nuse (\n\t./app\n\t./lib\n)\n")
	t.Setenv("GOWORK", goWork)

	project, err := jsonshape.Load(jsonshape.Config{Dir: appDir, ModulePath: "example.com/app", Roots: []string{"example.com/app/model"}})
	require.NoError(t, err)

	pkg := project.Package("example.com/app/model")
	require.NotNil(t, pkg)
	item, ok := pkg.Types.Scope().Lookup("Item").(*types.TypeName)
	require.True(t, ok)
	st, ok := item.Type().Underlying().(*types.Struct)
	require.True(t, ok)
	fields := project.Fields(st, jsonshape.Site{Subject: "model.Item", Pos: item.Pos()})
	require.Len(t, fields, 1)
	require.Equal(t, "name", fields[0].Key)
	require.Equal(t, "example.com/lib.Name", fields[0].Var.Type().String())
}

// writeFile writes content to path, creating its parent directories.
func writeFile(t *testing.T, path string, content string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}
