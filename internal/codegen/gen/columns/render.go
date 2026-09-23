package columns

import (
	"fmt"
	"go/format"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/ggconst"
)

// renderColumnsFile builds the generated source for one model source file.
func renderColumnsFile(module string, pkgName string, source string, models []modelColumns) (string, error) {
	imports := map[string]string{ggconst.ImportPathGst: "gst"}
	for _, m := range models {
		for _, col := range m.Columns {
			// A TimeColumn reference carries no type argument, so the column
			// type's import would be unused in the generated file.
			if col.Time && col.TypeExpr != "" {
				continue
			}
			if col.TypePkg == "" {
				continue
			}
			dot := strings.Index(col.TypeExpr, ".")
			if dot <= 0 {
				return "", errors.Newf("model %s column %q has import %q but its type %q carries no package qualifier",
					m.Name, col.DBName, col.TypePkg, col.TypeExpr)
			}
			alias := col.TypeExpr[:dot]
			if existing, ok := imports[col.TypePkg]; ok && existing != alias {
				return "", errors.Newf("model %s column %q needs import %q as %q but it is already imported as %q",
					m.Name, col.DBName, col.TypePkg, alias, existing)
			}
			for path, other := range imports {
				if other == alias && path != col.TypePkg {
					return "", errors.Newf("model %s column %q imports %q as %q, colliding with %q; add an explicit gorm column type or rename the package",
						m.Name, col.DBName, col.TypePkg, alias, path)
				}
			}
			imports[col.TypePkg] = alias
		}
	}

	// Standard library imports go in their own group, as gofmt convention
	// expects; format.Source keeps groups but does not create them. A path is
	// standard library only when its first segment carries no dot and it does
	// not belong to the project module, whose name may also be dotless.
	stdlib := make([]string, 0, len(imports))
	external := make([]string, 0, len(imports))
	for path := range imports {
		if isStdlibImport(path, module) {
			stdlib = append(stdlib, path)
		} else {
			external = append(external, path)
		}
	}
	sort.Strings(stdlib)
	sort.Strings(external)

	var buf strings.Builder
	buf.WriteString(consts.CodeGeneratedComment())
	fmt.Fprintf(&buf, "\n// source: %s\n\npackage %s\n\nimport (\n", filepath.ToSlash(source), pkgName)
	for _, path := range stdlib {
		buf.WriteString(importSpec(imports[path], path))
	}
	if len(stdlib) > 0 && len(external) > 0 {
		buf.WriteString("\n")
	}
	for _, path := range external {
		buf.WriteString(importSpec(imports[path], path))
	}
	buf.WriteString(")\n")

	for _, m := range models {
		fmt.Fprintf(&buf, "\n// %s are the typed column references of %s.\n", columnVarName(m.Name), m.Name)
		fmt.Fprintf(&buf, "var %s = struct {\n", columnVarName(m.Name))
		for _, col := range m.Columns {
			fmt.Fprintf(&buf, "\t%s %s", col.GoName, columnRefType(col))
			if col.TypeExpr == "" {
				fmt.Fprintf(&buf, " // %s", col.TypeName)
			}
			buf.WriteString("\n")
		}
		buf.WriteString("}{\n")
		for _, col := range m.Columns {
			fmt.Fprintf(&buf, "\t%s: %s,\n", col.GoName, columnRefLiteral(m.Name, col))
		}
		buf.WriteString("}\n")
	}

	formatted, err := format.Source([]byte(buf.String()))
	if err != nil {
		return "", errors.Wrapf(err, "format generated columns for %s", source)
	}
	return string(formatted), nil
}

// isStdlibImport reports whether an import path belongs to the standard
// library rather than to a dependency or to the project itself: in module
// tmpapp, time and math/rand/v2 do, while github.com/hydroan/gst and
// tmpapp/model/sample do not.
func isStdlibImport(path string, module string) bool {
	if module != "" && (path == module || strings.HasPrefix(path, module+"/")) {
		return false
	}
	return !strings.Contains(strings.SplitN(path, "/", 2)[0], ".")
}

// importSpec renders one import line of a generated file. The package name is
// written only when it cannot be inferred from the path: gofmt never spells a
// name it can read off the last segment, and a redundant one reads as if the
// package were called something else. A versioned path such as .../v2, or a
// package whose name differs from its directory, keeps the name, without which
// the generated file would not compile.
func importSpec(name, path string) string {
	if name == path[strings.LastIndex(path, "/")+1:] {
		return fmt.Sprintf("\t%q\n", path)
	}
	return fmt.Sprintf("\t%s %q\n", name, path)
}

// columnVarName returns the name of the var holding a model's generated
// column references: RecordCols for Record.
func columnVarName(model string) string {
	return model + "Cols"
}

// columnRefType returns the declared type of one generated column reference.
// Numeric and time columns get the specialized references that carry the
// aggregate functions only meaningful there; every other column, including one
// whose type cannot be written as source, gets the plain reference.
func columnRefType(col columnInfo) string {
	switch {
	case col.Time && col.TypeExpr != "":
		return "gst.TimeColumn"
	case col.Numeric && col.TypeExpr != "":
		return fmt.Sprintf("gst.NumericColumn[%s]", col.TypeExpr)
	default:
		return fmt.Sprintf("gst.Column[%s]", columnTypeParam(col))
	}
}

// columnRefLiteral returns the constructor call initializing one generated
// column reference. Construction goes through the NewXxx constructors rather
// than composite literals because the fields are unexported: a generated
// reference cannot be repointed at another column at run time. The model is
// the first type argument: the constructor reads TableName from a fresh value
// of it at package initialization, so the table name is never restated as a
// literal that could drift from the model.
func columnRefLiteral(model string, col columnInfo) string {
	switch {
	case col.Time && col.TypeExpr != "":
		return fmt.Sprintf("gst.NewTimeColumn[*%s](%q)", model, col.DBName)
	case col.Numeric && col.TypeExpr != "":
		return fmt.Sprintf("gst.NewNumericColumn[*%s, %s](%q)", model, col.TypeExpr, col.DBName)
	default:
		return fmt.Sprintf("gst.NewColumn[*%s, %s](%q)", model, columnTypeParam(col), col.DBName)
	}
}

// columnTypeParam returns the type argument for a column reference, falling
// back to any when the column type cannot be written as source.
func columnTypeParam(col columnInfo) string {
	if col.TypeExpr == "" {
		return "any"
	}
	return col.TypeExpr
}
