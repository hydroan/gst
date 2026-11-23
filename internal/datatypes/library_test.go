package datatypes_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"testing"

	"github.com/hydroan/gst/internal/datatypes"
	"github.com/hydroan/gst/internal/testutil/golist"
	"github.com/stretchr/testify/require"
)

// The accounts upstreamNames gives of a name the package has no name of its
// own for.
const (
	inDatabase  = "used by the JSON membership filter of the database package"
	noQuery     = "not offered: a query reaches into a document through the framework's filters and column references"
	noGenerator = "not offered: no generator of the framework writes the type"
	noNull      = "not offered: an optional column is a pointer field"
)

// upstreamNames maps every name gorm.io/datatypes exports to what the
// framework makes of it: the name of the package written in its place (see
// datatypes.Replacement), the package using it, or why a model is offered
// nothing for it.
var upstreamNames = map[string]string{
	"Date":         "Date",
	"Time":         "TimeOfDay",
	"NewTime":      "NewTimeOfDay",
	"JSON":         "JSON[json.RawMessage]",
	"JSONType":     "JSON",
	"NewJSONType":  "NewJSON",
	"JSONSlice":    "JSONSlice",
	"NewJSONSlice": "NewJSONSlice",
	"JSONMap":      "JSONMap",

	"JSONArrayQuery":      inDatabase,
	"JSONArrayExpression": inDatabase,

	"JSONQuery":              noQuery,
	"JSONQueryExpression":    noQuery,
	"JSONSet":                noQuery,
	"JSONSetExpression":      noQuery,
	"JSONOverlaps":           noQuery,
	"JSONOverlapsExpression": noQuery,
	"Column":                 noQuery,

	"UUID":              noGenerator,
	"NewUUIDv1":         noGenerator,
	"NewUUIDv4":         noGenerator,
	"BinUUID":           noGenerator,
	"NewBinUUIDv1":      noGenerator,
	"NewBinUUIDv4":      noGenerator,
	"NewNilBinUUID":     noGenerator,
	"BinUUIDFromString": noGenerator,
	"URL":               noGenerator,

	"Null":        noNull,
	"NewNull":     noNull,
	"NullBool":    noNull,
	"NullByte":    noNull,
	"NullFloat64": noNull,
	"NullInt16":   noNull,
	"NullInt32":   noNull,
	"NullInt64":   noNull,
	"NullString":  noNull,
	"NullTime":    noNull,
}

// TestEveryNameOfTheLibraryIsAccountedFor holds upstreamNames to what
// gorm.io/datatypes exports, read from the source of the version the module
// requires: a type or a function the library gains fails the test until the
// table says what the framework makes of it, and so does a name of the
// table the library exports no more. It holds datatypes.Replacement to the
// table as well: the name written in place of each name the table gives one
// for, and none for a name it gives a reason for.
func TestEveryNameOfTheLibraryIsAccountedFor(t *testing.T) {
	library := golist.Packages(t, "gorm.io/datatypes")[0]
	var exported []string
	fset := token.NewFileSet()
	for _, name := range library.GoFiles(t) {
		file, err := parser.ParseFile(fset, filepath.Join(library.Dir, name), nil, parser.SkipObjectResolution)
		require.NoError(t, err)
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && d.Name.IsExported() {
					exported = append(exported, d.Name.Name)
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						if s.Name.IsExported() {
							exported = append(exported, s.Name.Name)
						}
					case *ast.ValueSpec:
						for _, id := range s.Names {
							if id.IsExported() {
								exported = append(exported, id.Name)
							}
						}
					}
				}
			}
		}
	}
	slices.Sort(exported)

	notOffered := []string{inDatabase, noQuery, noGenerator, noNull}
	for _, name := range exported {
		require.Contains(t, upstreamNames, name, "gorm.io/datatypes exports %s, which upstreamNames gives no account of", name)
		account := upstreamNames[name]
		own, replaced := datatypes.Replacement(name)
		if slices.Contains(notOffered, account) {
			require.False(t, replaced, "upstreamNames offers nothing for %s, which Replacement answers %s for", name, own)
			continue
		}
		require.True(t, replaced, "upstreamNames writes %s in place of %s, which Replacement has nothing for", account, name)
		require.Equal(t, account, own, "the name written in place of %s", name)
	}
	for name := range upstreamNames {
		require.Contains(t, exported, name, "upstreamNames accounts for %s, which gorm.io/datatypes does not export", name)
	}
}
