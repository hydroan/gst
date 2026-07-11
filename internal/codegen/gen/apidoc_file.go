package gen

import (
	"fmt"
	"go/format"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/hydroan/gst/apidoc"
	"github.com/hydroan/gst/internal/codegen/constants"
	"github.com/hydroan/gst/types/consts"
)

// StructDocEntry describes the doc comments of one struct extracted from a
// model source file, identified by its package path and type name.
type StructDocEntry struct {
	PkgPath  string
	TypeName string
	Doc      apidoc.StructDoc
}

// EnumDocEntry describes one enum-like named type extracted from a model
// package: its doc comment and declared constant values.
type EnumDocEntry struct {
	PkgPath  string
	TypeName string
	Doc      apidoc.EnumDoc
}

// APIDocEntries bundles everything registered by the generated apidoc.go.
type APIDocEntries struct {
	Structs []StructDocEntry
	Enums   []EnumDocEntry
}

// BuildAPIDocFile generates an apidoc.go file that registers struct and field
// doc comments into the apidoc registry at build time, so the OpenAPI document
// keeps schema descriptions in binaries deployed without Go source files.
// The content looks like below:
/*
package model

import "github.com/hydroan/gst/apidoc"

func init() {
	apidoc.Register("myproject/model", "User", apidoc.StructDoc{
		Comment: "User is the user record.",
		Fields: map[string]string{
			"Name": "Name is the user name.",
		},
	})
}
*/
func BuildAPIDocFile(pkgName string, entries APIDocEntries) (string, error) {
	src, err := format.Source([]byte(buildAPIDocSource(pkgName, entries)))
	if err != nil {
		return "", err
	}
	return string(src), nil
}

// buildAPIDocSource assembles the unformatted apidoc.go source code.
func buildAPIDocSource(pkgName string, entries APIDocEntries) string {
	var b strings.Builder
	b.WriteString(consts.CodeGeneratedComment())
	b.WriteString("\n\n")
	b.WriteString("package " + pkgName + "\n\n")

	// If there are no entries, the init function body is empty,
	// so we should not import any external package.
	if len(entries.Structs) > 0 || len(entries.Enums) > 0 {
		b.WriteString("import " + strconv.Quote(constants.ImportPathAPIDoc) + "\n\n")
	}

	b.WriteString("func init() {\n")
	for _, entry := range entries.Structs {
		b.WriteString("\tapidoc.Register(" + strconv.Quote(entry.PkgPath) + ", " + strconv.Quote(entry.TypeName) + ", apidoc.StructDoc{\n")
		if entry.Doc.Comment != "" {
			b.WriteString("\t\tComment: " + strconv.Quote(entry.Doc.Comment) + ",\n")
		}
		if len(entry.Doc.Fields) > 0 {
			b.WriteString("\t\tFields: map[string]string{\n")
			for _, name := range slices.Sorted(maps.Keys(entry.Doc.Fields)) {
				b.WriteString("\t\t\t" + strconv.Quote(name) + ": " + strconv.Quote(entry.Doc.Fields[name]) + ",\n")
			}
			b.WriteString("\t\t},\n")
		}
		b.WriteString("\t})\n")
	}
	for _, entry := range entries.Enums {
		b.WriteString("\tapidoc.RegisterEnum(" + strconv.Quote(entry.PkgPath) + ", " + strconv.Quote(entry.TypeName) + ", apidoc.EnumDoc{\n")
		if entry.Doc.Comment != "" {
			b.WriteString("\t\tComment: " + strconv.Quote(entry.Doc.Comment) + ",\n")
		}
		b.WriteString("\t\tValues: []apidoc.EnumValue{\n")
		for _, value := range entry.Doc.Values {
			b.WriteString("\t\t\t{Value: " + enumValueLiteral(value.Value))
			if value.Comment != "" {
				b.WriteString(", Comment: " + strconv.Quote(value.Comment))
			}
			b.WriteString("},\n")
		}
		b.WriteString("\t\t},\n")
		b.WriteString("\t})\n")
	}
	b.WriteString("}\n")

	return b.String()
}

// enumValueLiteral renders one enum constant value as a Go literal.
func enumValueLiteral(value any) string {
	switch v := value.(type) {
	case string:
		return strconv.Quote(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}
