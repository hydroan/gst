package gen

import (
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
func BuildAPIDocFile(pkgName string, entries []StructDocEntry) (string, error) {
	src, err := format.Source([]byte(buildAPIDocSource(pkgName, entries)))
	if err != nil {
		return "", err
	}
	return string(src), nil
}

// buildAPIDocSource assembles the unformatted apidoc.go source code.
func buildAPIDocSource(pkgName string, entries []StructDocEntry) string {
	var b strings.Builder
	b.WriteString(consts.CodeGeneratedComment())
	b.WriteString("\n\n")
	b.WriteString("package " + pkgName + "\n\n")

	// If there are no entries, the init function body is empty,
	// so we should not import any external package.
	if len(entries) > 0 {
		b.WriteString("import " + strconv.Quote(constants.ImportPathAPIDoc) + "\n\n")
	}

	b.WriteString("func init() {\n")
	for _, entry := range entries {
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
	b.WriteString("}\n")

	return b.String()
}
