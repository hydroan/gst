package codegenast

import (
	"go/ast"
	"slices"
	"strconv"
)

// PackageNames is how one file refers to an imported package: through the
// qualifiers it imports the package under, and without a qualifier when it
// dot-imports the package.
type PackageNames struct {
	Qualifiers  []string
	DotImported bool
}

// ImportedNames returns how file refers to the package at importPath: the
// alias of every import of the package, or defaultName for an import
// without one, and whether one of the imports is a dot import. A blank
// import names nothing. For a file importing
//
//	import (
//		"github.com/hydroan/gst/model"
//		gstmodel "github.com/hydroan/gst/model"
//		. "github.com/hydroan/gst/model"
//		_ "github.com/hydroan/gst/model"
//	)
//
// ImportedNames(file, "github.com/hydroan/gst/model", "model") returns the
// qualifiers model and gstmodel with DotImported set; for a file that does
// not import the package it returns the zero PackageNames.
func ImportedNames(file *ast.File, importPath, defaultName string) PackageNames {
	var names PackageNames
	for _, imp := range file.Imports {
		if imp.Path == nil {
			continue
		}
		if path, err := strconv.Unquote(imp.Path.Value); err != nil || path != importPath {
			continue
		}
		switch {
		case imp.Name == nil:
			names.Qualifiers = append(names.Qualifiers, defaultName)
		case imp.Name.Name == ".":
			names.DotImported = true
		case imp.Name.Name != "_":
			names.Qualifiers = append(names.Qualifiers, imp.Name.Name)
		}
	}
	return names
}

// Refers reports whether expr names one of the package's types typeNames:
// a selector qualified by one of the qualifiers, or a bare name under a dot
// import. With the qualifier gstmodel, gstmodel.Base refers to Base and a
// bare Base does not, being a type of the file's own package; under a dot
// import the bare Base refers to it too.
func (n PackageNames) Refers(expr ast.Expr, typeNames ...string) bool {
	switch t := expr.(type) {
	case *ast.SelectorExpr:
		ident, ok := t.X.(*ast.Ident)
		return ok && slices.Contains(n.Qualifiers, ident.Name) && slices.Contains(typeNames, t.Sel.Name)
	case *ast.Ident:
		return n.DotImported && slices.Contains(typeNames, t.Name)
	}
	return false
}
