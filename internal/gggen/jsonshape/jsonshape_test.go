package jsonshape_test

import (
	"go/types"
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/gggen/jsonshape"
	"github.com/stretchr/testify/require"
)

// fixtureModule is the module path the fixture packages stand in for a
// project module under. They are kept here for every generator that reads
// shapes off a project; the TypeScript golden files render these very shapes.
const fixtureModule = "github.com/hydroan/gst/internal/gggen/jsonshape/fixture"

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
