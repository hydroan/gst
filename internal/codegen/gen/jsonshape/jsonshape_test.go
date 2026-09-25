package jsonshape_test

import (
	"go/types"
	"testing"

	"github.com/hydroan/gst/internal/codegen/gen/jsonshape"
	"github.com/stretchr/testify/require"
)

// fixtureModule is the module path the fixture packages of the TypeScript
// generator stand in for a project module under; the shapes read off them
// are the ones its golden files render.
const fixtureModule = "github.com/hydroan/gst/internal/codegen/gen/ts/fixture"

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
