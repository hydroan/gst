package ggmodule

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"

	"github.com/hydroan/gst/internal/gghelper"
)

func parseGoFile(path string) (*token.FileSet, *ast.File, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	return fset, file, err
}

func writeGoFile(path string, fset *token.FileSet, file *ast.File, modulePath string) error {
	formatted, err := gghelper.FormatNodeExtraWithFileSet(file, fset, modulePath)
	if err != nil {
		return err
	}
	if err := gghelper.EnsureParentDir(path); err != nil {
		return err
	}
	// #nosec G703 -- every caller passes a path joined from the project
	// middleware directory and a constant filename, validated by
	// gghelper.PathUnderRoot where plan data is involved.
	return os.WriteFile(path, []byte(formatted), 0o600)
}

func identName(ident *ast.Ident) string {
	if ident == nil {
		return ""
	}
	return ident.Name
}
