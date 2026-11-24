package ggcheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strings"

	"github.com/hydroan/gst/internal/datatypes"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/goast"
)

// GormDatatypesBan keeps the names of gorm.io/datatypes the framework has
// its own for out of project code.
var GormDatatypesBan = Check{
	Name: "Gorm datatypes ban",
	Rule: "project code must not name the Date, Time, JSON, JSONMap, JSONType or JSONSlice of gorm.io/datatypes, nor the functions making them: the types of github.com/hydroan/gst/datatypes stand in their place, and a day, a time of day and a JSON document are sent, stored and answered the one way the framework tells by those alone; the rest of the library, its JSON queries among it, is a project's to use",
	run:  checkGormDatatypesBan,
}

// gormDatatypesName is the name a file knows gorm.io/datatypes by when it
// imports it under none, the one of its package clause.
const gormDatatypesName = "datatypes"

// checkGormDatatypesBan reports every place project code names a type or a
// function of gorm.io/datatypes the framework has its own for (see
// datatypes.Replacement), with the name to write instead. A column declared
// by such a type is refused when its table is prepared; anywhere else, a
// request or a response type among it, the project compiles, generates and
// starts, and the value is written and read the library's way: its Date as
// the instant of a midnight, 2026-01-02T00:00:00Z, refusing the 2026-01-02
// the framework's is written as, its Time reading a text it cannot read as
// midnight. Both packages are named datatypes, so the wrong one is an
// import away.
//
// The check is syntactic: a name is one qualified by what the file imports
// the library under. A file importing it under a dot is reported as such,
// its names not to be told apart from the file's own. Test files are left
// alone, and so are the files gg generates: gg gen runs the check before it
// writes them, and one written from a model the project has since moved to
// the framework's types names the library until that run replaces it.
func checkGormDatatypesBan(ignore gghelper.ProjectIgnore) []string {
	var violations []string
	err := ignore.Walk(".", func(path string, info os.FileInfo) error {
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ggconst.ExtensionGo) || strings.HasSuffix(path, ggconst.SuffixTestFile) || isGeneratedFileName(path) {
			return nil
		}
		violations = append(violations, checkFileGormDatatypes(path)...)
		return nil
	})
	if err != nil {
		violations = append(violations, fmt.Sprintf("walking project directory: %v", err))
	}
	return violations
}

// checkFileGormDatatypes reports the names of gorm.io/datatypes one file
// writes that the framework has its own for.
func checkFileGormDatatypes(filePath string) []string {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil
	}
	relPath := gghelper.RelativePath(filePath)

	var violations []string
	for _, spec := range file.Imports {
		if spec.Name != nil && spec.Name.Name == "." && spec.Path.Value == `"`+datatypes.LibraryPath+`"` {
			violations = append(violations, fmt.Sprintf(
				"%s:%d: dot-imports %s; the Gorm datatypes ban cannot tell its names apart, import it under a name",
				relPath, fset.Position(spec.Pos()).Line, spec.Path.Value,
			))
		}
	}
	qualifiers := goast.ImportedNames(file, datatypes.LibraryPath, gormDatatypesName).Qualifiers
	if len(qualifiers) == 0 {
		return violations
	}
	ast.Inspect(file, func(n ast.Node) bool {
		selector, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		qualifier, ok := selector.X.(*ast.Ident)
		if !ok || !slices.Contains(qualifiers, qualifier.Name) {
			return true
		}
		own, replaced := datatypes.Replacement(selector.Sel.Name)
		if !replaced {
			return true
		}
		violations = append(violations, fmt.Sprintf(
			"%s:%d: %s.%s is one the framework has its own for; write datatypes.%s of %s",
			relPath, fset.Position(selector.Pos()).Line, datatypes.LibraryPath, selector.Sel.Name, own, ggconst.ImportPathDatatypes,
		))
		return true
	})
	return violations
}
